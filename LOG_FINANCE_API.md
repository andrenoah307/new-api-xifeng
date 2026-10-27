# Root Log and Finance APIs

These APIs use the existing administrator authentication headers or session.
Root access is checked against the current database role and enabled status on
every request, including cached statistics. They do not change billing or store
channel cost settings. Ordinary administrator log pages remain capped at 100.

## Bulk log reads

`GET /api/log/?page_size=10000&count=false&start_timestamp=...&end_timestamp=...`

- Root page size defaults to a maximum of 10000. `ROOT_LOG_PAGE_SIZE` can lower
  that maximum, within 100 through 10000. Request the desired `page_size` explicitly.
- `count=false` avoids the exact total count and OFFSET. The response contains
  `total: -1`, `has_more`, and a string `next_before_id`. Pass that string as
  `before_id` on the next request and keep all filters and timestamps fixed.
- Cursor requests use page 1 and SQL primary-key descending order. Stop when
  `has_more` is false. This mode is unavailable for ClickHouse display IDs.
- Existing log endpoints retain their timestamp semantics and default range.
  Set an explicit start timestamp for older data. Concurrent deletion or backfill
  means pagination is not a database snapshot.
- Larger pages are for ledger export, not downloading hundreds of millions of
  records into browser storage. Existing query timeouts still apply.

## Resumable channel aggregation

`GET /api/log/finance/stat?start_timestamp=...&end_timestamp=...&window_seconds=3600`

Optional `channel` filters one channel; omitted or zero means all channels.
The interval is **start inclusive, end exclusive**. Start is required; end defaults
to a minute boundary at least two minutes behind the present. Future ends are rejected.

Each call processes at most `window_seconds` (default one hour, range 1–86400).
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

## Performance and Compatibility

- Aggregation runs on the source database and returns channel totals, not logs.
  It works with SQL log stores and ClickHouse without relying on display IDs.
- `ROOT_LOG_STAT_TIMEOUT` defaults to 180 seconds and is capped at 600 seconds.
  Client/proxy timeouts must accommodate this, or clients should use smaller windows.
- One aggregation runs per application process; competing requests receive 503
  with `Retry-After: 3`. The last identical result is reused for one minute.
  This is neither cluster-wide locking nor durable preaggregation. A cached result
  can omit late writes until expiry; refresh affected windows to reconcile them.
- MySQL all-channel queries use an existing type/time index when present. Channel
  queries let the optimizer select a channel index. No index or table is created.
- PostgreSQL 9.6 cannot safely cast arbitrary malformed JSON text: malformed `other`
  fails the window rather than returning silently incomplete totals. Other supported
  dialects mark malformed pricing metadata as missing basis.
- More than 10000 pricing groups fails the window instead of truncating totals.
  This bound limits output, not scan work. A cold historical scan remains proportional
  to data volume. Subsecond full-history statistics require a separately designed
  incremental rollup with historical pricing and refund attribution; lifting API
  limits or switching engines alone does not guarantee that performance.
- Existing log query rate middleware already exempts administrators and root.
  A client-side delay is independent of that exemption. Global API protection remains.

The external finance client must adopt this endpoint and window persistence to use
aggregated synchronization. Deploying this source change alone does not switch an
existing client away from its raw-log synchronization workflow.
