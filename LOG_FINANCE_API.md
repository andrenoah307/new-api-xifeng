# Log and Finance APIs

These APIs use the existing administrator authentication headers or session.
Finance access is checked against the current database role and enabled status on
every request, including cached statistics. They do not change billing or store
channel cost settings. Ordinary administrator log pages remain capped at 100.

## Bulk log reads

`GET /api/log/?page_size=10000&count=false&start_timestamp=...&end_timestamp=...`

- Root page size defaults to a maximum of 10000. `ROOT_LOG_PAGE_SIZE` can lower
  that maximum, within 100 through 10000. Request the desired `page_size` explicitly.
- For SQL stores, `count=false` avoids the exact total count and OFFSET. The response contains
  `total: -1`, `has_more`, and a string `next_before_id`. Pass that string as
  `before_id` on the next request and keep all filters and timestamps fixed.
- SQL cursor requests use page 1 and primary-key descending order. Stop when
  `has_more` is false. ClickHouse uses `pagination=cursor` and opaque `next_cursor`
  tokens passed back as `cursor`; it must not use display IDs or `before_id`.
  Root `count=false` automatically selects that ClickHouse cursor path.
- Existing log endpoints retain their timestamp semantics and default range.
  Set an explicit start timestamp for older data. Concurrent deletion or backfill
  means pagination is not a database snapshot.
- Larger pages are for ledger export, not downloading hundreds of millions of
  records into browser storage. Existing query timeouts still apply.

## Resumable channel aggregation

`GET /api/log/finance/stat?start_timestamp=...&end_timestamp=...&granularity=month`

Administrators (role >= 10) can query channel totals. Ordinary authenticated users
use `GET /api/log/self/finance/stat` with the same time parameters. The personal
endpoint forces the authenticated user ID, ignores any supplied `user_id`, rejects
channel filters and returns `personal` instead of `channels`. Its only financial
fields are `consume_quota`, `refund_quota`, `net_quota`, `consume_count`, and
`refund_count`: no channel identity, cost basis or channel profit is exposed.

Optional `channel` filters one channel; omitted or zero means all channels.
The interval is **start inclusive, end exclusive**. Start is required; end defaults
to a minute boundary at least two minutes behind the present. Future ends are rejected.

The default `granularity=day` processes at most `window_seconds` (default one day,
range 1–86400). `granularity=month` targets the next UTC month boundary, bounded by
the requested end. Monthly queries roll up published daily snapshots; they do not
download log records. Missing/stale summaries and exact partial-day edges use raw
logs, at most one raw UTC day per call, so the actual end may be earlier.
The response contains the actual `start_timestamp` and `end_timestamp`, the fixed
`requested_end_timestamp`, `complete`, and `next_start_timestamp`. Continue with
that next start and the original requested end until complete. Empty windows also
advance. On timeout, retry the same uncommitted window with a smaller window size.
Never advance on an error. Persist each window's result and cursor atomically,
replacing a retried window instead of adding it twice. Progress can show completed
time range and accumulated `consume_count`/`refund_count`; elapsed time coverage is
not a percentage of rows because traffic density varies.

Amounts are decimal **quota strings**, not currency or JavaScript floating point:

| Field | Meaning |
| --- | --- |
| `consume_quota` | Recorded consumption, already including historical group multiplier |
| `refund_quota` | Recorded refunds in this interval |
| `net_quota` | Consumption minus refunds |
| `known_model_quota` | Sum of consumption divided by valid historical `group_ratio` |
| `missing_basis` | Consumption records with unusable or saturated pricing provenance |
| `unresolved_refunds` | Refund records whose original cost has not been attributed |

For a constant channel cost multiplier, known consumption cost is
`known_model_quota * channel_multiplier`. Split windows at multiplier effective
times when historical rates differ. Convert quota to currency using the source's
configured quota conversion. This basis reflects rounding in the recorded charge.
Do not label partial known cost as complete profit when `missing_basis > 0` or
refund cost is unresolved. Refund cost needs the original consumption and its
historical channel multiplier; the refund's current multiplier is insufficient.
`refund_cost_resolved` is true only when the window contains no unresolved refunds.

## ClickHouse Daily Snapshots and Monthly Reports

ClickHouse supports incremental and refreshable materialized views. This project
uses an explicit bounded snapshot builder: incremental insert-triggered views do
not see historical `ATTACH`/`REPLACE PARTITION` imports and cannot retract corrected
records. A single full-history refresh would also create unbounded background work.
Daily snapshots permit precise partial dates and changing channel rates; a monthly
request sums those days, without maintaining a second monthly copy to reconcile.

Build the offline tool and initialize the auxiliary table explicitly:

```sh
go build -o bin/clickhouse-finance ./cmd/clickhouse-finance
# Set LOG_FINANCE_CLICKHOUSE_DSN through your deployment's secret environment.
./bin/clickhouse-finance -init
./bin/clickhouse-finance -start 2026-09-01 -end 2026-10-01
```

- `-start` is inclusive, `-end` exclusive, both UTC midnight. Each invocation is
  limited to 31 completed days. Current-day data stays live. `-day-timeout` defaults
  to 10 minutes and is bounded to one hour. Queries use at most two threads and
  2 GiB memory, spilling aggregation after 256 MiB. Monitor disk and workload before
  scheduling historical backfills. There is no automatic startup backfill.
- Each day is grouped by user, channel, log type and historical group-ratio
  provenance. Integer amounts use `Int128`; API amounts are decimal strings.
  A marker records coverage even when a day has no transactions.
- Publishing replaces one daily partition atomically. Retrying the same day
  replaces rather than adds it. Readers see either complete old or complete new
  partitions. Parallel builders for the same day are refused by a database gate.
- A canceled/crashed builder may retain `logs_finance_lock_<UTC epoch>` and a
  `logs_finance_stage_<random>` table. Inspect active server queries and the named
  staging table before removing either; do not steal locks by age or remove all
  finance tables. Successful or ordinary failed work cleans up its own tables.
- Freshness is checked against the source table UUID and the active source month's
  part names and checksums, before and after reading summaries. Historical imports,
  partition replacement, mutations and table replacement invalidate stale results.
  Background merges also conservatively invalidate snapshots. Rebuild affected
  completed days after such changes; no TTL allows stale snapshots to appear current.
- This fingerprint is month-scoped: active writes into the current month can
  invalidate that month's completed-day snapshots. Closed, settled historical
  months are the intended fast path. This is not a claim of incremental real-time
  materialization or a cross-request transactional snapshot.
- GET requests never create or rebuild tables. A missing summary table uses the
  existing bounded raw aggregate path. Configure the source application's
  ClickHouse account for `SELECT` on the summary table and `system.parts`/
  `system.tables`; the separate builder requires create/insert/alter/drop rights
  for its auxiliary tables. The builder never writes `logs`.
- Responses expose `storage` (`raw`, `clickhouse_daily`, `mixed`), `summary_days`,
  `raw_days`, and `summary_generated_at` (oldest used snapshot, zero if none).
  A completed report covers its requested time interval as queried, not a promise
  that delayed queue events can never arrive later. Refresh affected intervals to
  reconcile late arrivals. Historical baseline division rounds to 18 decimal places
  per pricing group and daily interval.

Monthly client requests must still split at channel-multiplier effective times.
Do not substitute an entire day's cost for a partial-day rate interval. On timeout,
retry the same start with `granularity=day` and a smaller `window_seconds`.

### Scheduling

An operator-managed timer can run the following command from a service that already
receives `LOG_FINANCE_CLICKHOUSE_DSN` through a protected environment file. This GNU
date example rebuilds the previous closed month; schedule it after historical imports
and source merges have settled, then rerun affected months after corrections:

```sh
finance_end=$(date -u +%Y-%m-01)
finance_start=$(date -u -d "$finance_end -1 month" +%Y-%m-%d)
/path/to/clickhouse-finance -start "$finance_start" -end "$finance_end"
```

Optionally rebuild just yesterday nightly:

```sh
/path/to/clickhouse-finance -start "$(date -u -d yesterday +%Y-%m-%d)" -end "$(date -u +%Y-%m-%d)"
```

It is not a substitute for closing the month: new source writes and merges in that same month
can conservatively invalidate the previous day's summary. Avoid multiple overlapping
timers, retain failed-day output, and resume at the reported incomplete day. The
tool publishes a progress line only after each daily partition is committed.

## Performance and Compatibility

### Query Queue and Cancellation

Statistics requests enter a process-local FIFO queue: one running query, at most
32 active requests including waiters, and at most two per authenticated user.
Supply a unique UUID `task_id` on each aggregate request to observe it while the
original HTTP request waits. Existing synchronous aggregate responses are retained.

- `GET /api/log/finance/tasks?task_id=<uuid>` returns the caller's active tasks
  (`id`, `state`, `ahead`, `created_at`). Omit the ID to list that caller's tasks.
  States are `queued`, `running`, and `cancelling`; `ahead` includes the running
  task. No other user's IDs or query details are exposed, even to administrators.
- `POST /api/log/finance/tasks/cancel?task_id=<uuid>` cancels only a task owned by
  the caller. The response contains `cancelled`; false also covers missing or
  already completed tasks. Current account status is checked for both endpoints.
- Cancelling a waiter removes it immediately. Cancelling a running query signals
  its database context but retains the execution slot until the query returns.
  The original statistics request returns `data: {cancelled: true, task_id: ...}`
  on explicit cancellation. Treat this as terminal, never retry or commit a window.
- Disconnecting or aborting the original request cancels its work. Tasks and
  results are not persisted; finished tasks disappear from the active list.
  Continue from the client's last committed window after a restart or disconnect.
- `ROOT_LOG_STAT_TIMEOUT` includes waiting and execution. A full queue or timeout
  still returns a bounded failure; increasing queue size is not a throughput fix.
- Multiple source replicas need sticky routing for status/cancel requests to reach
  the same process. This is not a durable or cluster-wide job scheduler. Backend
  proxies must propagate request cancellation. Permission is rechecked after
  queue admission and before returning a completed result.

### Validation

Dev CI starts `clickhouse/clickhouse-server:26.3.19.3`, verifies its reported
version and sets `TEST_CLICKHOUSE_DSN` for actual integration tests. Fixtures use
unique temporary databases and clean them up. Finance/queue tests also run with
the Go race detector; CI does not use production databases or credentials.

- Aggregation runs on the source database and returns channel totals, not logs.
  It works with SQL log stores and ClickHouse without relying on display IDs.
- `ROOT_LOG_STAT_TIMEOUT` defaults to 180 seconds and is capped at 600 seconds.
  Client/proxy timeouts must accommodate this, or clients should use smaller windows.
- One aggregation runs per application process; competing requests queue within
  the bounds above. Rejected admission receives 503 with `Retry-After: 3`.
  The last identical SQL result is reused for one minute,
  scoped by user ID. ClickHouse results bypass this cache to validate snapshot
  freshness each time. The online query gate is per-process, not cluster-wide.
- MySQL all-channel queries use an existing type/time index when present. Channel
  queries let the optimizer select a channel index. Personal queries retain the
  user filter and do not force the all-channel index. No SQL index/table is created.
- PostgreSQL 9.6 cannot safely cast arbitrary malformed JSON text: malformed `other`
  fails the window rather than returning silently incomplete totals. Other supported
  dialects mark malformed pricing metadata as missing basis.
- More than 10000 pricing groups fails the window instead of truncating totals.
  This bound limits output, not scan work. An initial historical snapshot build
  remains proportional to raw data volume; only subsequent valid snapshot reads
  avoid that scan. No production-scale latency guarantee is implied by local tests.
- Existing log query rate middleware already exempts administrators and root.
  A client-side delay is independent of that exemption. Global API protection remains.

The external finance client must adopt this endpoint and window persistence to use
aggregated synchronization. Deploying this source change alone does not switch an
existing client away from its raw-log synchronization workflow.
