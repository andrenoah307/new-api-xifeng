# ClickHouse log storage and Redis write queue

ClickHouse is supported only for `LOG_SQL_DSN`; the primary database remains
SQLite, MySQL or PostgreSQL. The optional services in `docker-compose.yml` use
internal service names and bind mounts. Change example passwords before use.
Use ClickHouse 26.3.19.3 or a version verified against the integration tests.
Startup selects the lightweight user projection on ClickHouse 25.5 and later;
older servers use a user bloom-filter index (verified on 25.3.14.14). The offline
`-print-schema` output targets 25.5+; it does not detect a server version.

With an empty `LOG_SQL_DSN`, logs reuse the primary database and no ClickHouse
migration runs. Startup loads `.env` from its working directory (`/data` in the
image), so an omitted environment variable can still be supplied by that file.
An explicitly empty `LOG_SQL_DSN` prevents this dotenv fallback. Check effective
container configuration and mounted files without publishing credentials.

## Schema and queries

Generate the application schema without connecting to a database:

```sh
go run ./cmd/clickhouse-log-import -print-schema
```

The schema preserves 22 historical fields, NULLs and signed integer widths,
adds private event/source/batch identities, and uses ZSTD(3). UTC month and
migration batch isolate imported partitions from live logs. Time/event ordering,
request/token skipping indexes and a lightweight user projection support log
queries. Existing incompatible tables are rejected, not automatically rewritten.

The admin and self log endpoints accept `pagination=cursor`, followed by
`cursor=<next_cursor>`. Results include `has_more`, `next_cursor`, and `total=-1`.
Keep filters and time bounds unchanged. This removes OFFSET/COUNT, but is not a
cross-request database snapshot. Existing offset clients remain supported and
must explicitly adopt cursors.

## Enable the durable queue

```dotenv
LOG_SQL_DSN=clickhouse://log_writer:password@clickhouse:9000/new_api_logs
LOG_SQL_CLICKHOUSE_TTL_DAYS=0
LOG_QUEUE_ENABLED=true
LOG_QUEUE_REDIS_URL=redis://:password@log-redis:6379
LOG_QUEUE_MAX_ROWS=1000000
LOG_QUEUE_MAX_BYTES=1073741824
```

The queue is opt-in. Without it, logs are written directly and failed writes are
not retained. With it, the application acknowledges enqueue after Redis accepts
the complete log; a worker writes ClickHouse in batches of at most 1,000 rows or
4 MiB, normally once per second. Each event is limited to 4 MiB. Accepted pending
and in-flight events count against both limits. Each event expires 86,400 seconds
after its first enqueue, using Redis time. New traffic and retries do not renew
older events. Undelivered events are discarded after this retention window;
this does not delete logs already stored in ClickHouse.
The byte limit measures JSON payload, not total Redis RSS or AOF disk use.

Cleanup is bounded to 1,000 pending events plus the in-flight batch per pass and
runs even when ClickHouse is unavailable. An active delivery lease defers batch
changes until it ends; cleanup can therefore lag the deadline. Shared keys also
expire at the latest event deadline, releasing an idle queue without a worker.
Pre-upgrade events receive a one-time 24-hour grace period on first access by
the new version. Upgrade all writers together.

`LOG_QUEUE_REDIS_URL` defaults to `REDIS_CONN_STRING`. A dedicated instance is
recommended to isolate queue memory/AOF I/O from sessions, billing and rate
limits. Redis must have AOF enabled, `maxmemory-policy=noeviction`, and
`appendfsync=always` or `everysec`, with `no-appendfsync-on-rewrite=no`;
CONFIG GET permission is required for startup
validation. The application never changes Redis persistence settings. `everysec`
can lose recent acknowledged writes in a host failure. Even `always` is not a
replicated backup and cannot protect against disk loss.

When Redis is unavailable, an event is oversized, or the queue is full, enqueue
returns an error and existing logging callers report it. They do not roll back
an already completed relay/billing operation. No unbounded in-memory fallback,
silent eviction, or direct ClickHouse fallback is used. Consequently this is
bounded durability for accepted events, not unconditional zero-loss accounting.
Monitor enqueue errors and capacity before saturation.

When ClickHouse is unavailable at startup, queue mode skips its connection and
migration checks in the foreground; the worker verifies/initializes it after
reconnection. Log queries can fail during the outage. Redis itself must be
available at startup. On shutdown the worker stops, leaving pending/in-flight
data in Redis for the next process. No backlog drain is required to stop safely.

## Delivery and recovery

All application nodes writing a target must use queue mode, the same Redis DB,
and identical ClickHouse DSN host/database path. These define the queue namespace;
changing them creates a different queue and strands the old backlog. Password
rotation does not change the namespace. Do not mix independent queues or direct
live writers for one destination.

Roll out queue mode to all live writers together. To disable it or change the
destination, first stop new enqueueing in a coordinated maintenance window and
verify both pending and in-flight counts are zero. Turning the flag off does not
drain or migrate an existing Redis backlog.

Lua atomically checks capacity, retains event identity/content, and moves pending
events into one immutable in-flight batch. Consumers use a 60-second owner lease;
each attempt has a 20-second context deadline. Acknowledgement checks both owner
and batch token before removing payloads/capacity. Expired consumers cannot
acknowledge another consumer's work. Failures back off up to 30 seconds; a failed
lease is allowed to expire before recovery.

The worker enables a 100,000-block non-replicated deduplication window and writes
the immutable batch synchronously with `insert_deduplication_token`. It verifies
existing event IDs and complete stored values before insertion: a fully matching
batch is acknowledged without reinsertion, including after dedup-window eviction.
Partial publication, changed contents, duplicate identities or corrupt payloads
stop that batch for operator reconciliation rather than risking double counting
or silently discarding records before their retention deadline. Later events
remain pending behind it. Expiration can remove blocked events; when it changes
a batch, the remaining events receive a new deduplication token after the lease
ends.

Redis stores the destination table UUID. Dropping/recreating the table requires
explicit reconciliation and rebinding; it must not silently replay a queue into
a different table. ClickHouse table TTL must be disabled in both configuration
and table schema; this is separate from Redis queue retention.
Do not mutate/delete live queued events, disable deduplication, erase queue keys,
restore Redis and ClickHouse to unrelated snapshots, or clear an in-flight batch
without reconciling its full contents. The event column alone is not a UNIQUE
constraint. Native deduplication is finite; arbitrary competing writers and
manual mutations are outside this recovery protocol.

Once per minute, application logs report pending/in-flight row count, payload
bytes, delivered count, capacity rejections and worker-cleaned expired count.
Counters are not a durable audit: native key expiration also removes state and
does not increment the expired counter. Keys use `logqueue:{hash}:` with
`pending`, `inflight`, `payload`, `expires`, `state`, and `lease`. State counters can be read
with HGETALL; payloads contain sensitive logs and must not be exposed publicly.
For a blocked batch, stop consumers, preserve a Redis backup, compare the exact
event identities and fields with ClickHouse, then repair the cause. Never use
DEL/FLUSHDB or trimming as automatic recovery.

## Historical imports

`cmd/clickhouse-log-import` reads an immutable JSONEachRow archive containing all
22 source fields and a JSON manifest with `Source`, `AfterID`, `ThroughID`,
`Month` (UTC YYYYMM), `Rows`, and `SHA256`.
IDs must be strictly ascending in `(AfterID, ThroughID]`, in one UTC month;
each batch is limited to one million rows. Example manifest:

```json
{"Source":"archive-1","AfterID":0,"ThroughID":100,"Month":202601,"Rows":100,"SHA256":"<64 hexadecimal characters>"}
```

```sh
LOG_IMPORT_CLICKHOUSE_DSN='clickhouse://user:password@clickhouse:9000/new_api_logs' \
  go run ./cmd/clickhouse-log-import -manifest manifest.json -archive logs.jsonl
```

The importer verifies the archive hash and full-field readback, rejects overlapping
source IDs, then atomically replaces an isolated batch partition. It never connects
to or deletes from the source database. Canceled imports retain their lock/staging
table until the operator confirms server queries have stopped. Allocate space for
staging, merges and growth; deleting MySQL rows does not shrink an ibd file, and
moving sort temporary files does not relocate an InnoDB intermediate rebuild table.

## Verification

```sh
TEST_CLICKHOUSE_DSN='clickhouse://127.0.0.1:9000/default' \
TEST_REDIS_URL='redis://127.0.0.1:6379' \
  go test ./model ./controller ./cmd/clickhouse-log-import -count=1
```

Use isolated test instances: integration tests create/drop databases and queue
fixtures. The optional `TestLogQueueRedisPersistence` has explicit `enqueue` and
`recover` phases (`TEST_QUEUE_PERSISTENCE_PHASE`) for testing a forced Redis
restart between phases. Never point these tests at production.
