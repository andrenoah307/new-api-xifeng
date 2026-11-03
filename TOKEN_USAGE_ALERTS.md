# API Key usage email alerts

## Configuration and quota semantics

The system settings page includes a default-disabled alert configuration in both
themes. Existing system-settings root authorization is retained. Enable Redis and
configure SMTP before enabling alerts. Mail goes to the current enabled owning
user's bound email address, never an address supplied by the API caller.

The `TokenUsageAlertRules` option supports up to 20 rules. Each rule has a stable
`id`, integer `percent` (1-100), `cooldown_days` (1-365), and a quota `scope`:

- `total`: `used_quota / (used_quota + remain_quota)` for finite total quotas.
- `period`: effective current-period usage divided by the configured period limit.
- `any`: check both, report the highest matching percentage in one email.

Default rules are 50% every seven days and 70% every three days. Thresholds are
inclusive. Rules with identical thresholds and overlapping scopes are rejected.
Each user/key/rule has an independent rolling cooldown. Keep IDs stable when editing
rules; deleting and recreating a rule creates a new policy identity. Configuration
edits do not shorten an existing persisted cooldown.

Period checks reuse the key's existing period calculation and Beijing-time
boundaries. Old-period usage is not carried into a new period. Unlimited total
quota keys may still trigger their finite period quota. Period rollover does not
reset mail cooldowns. Disabled, expired, deleted keys and disabled users are skipped.

This is the existing key's accounting usage, which may include in-flight
precharges. It is not a new settled-consumption ledger or an additional quota gate.
Refunds and edits are reflected by replacing snapshots from authoritative token
rows, not by incrementing consume-log totals. No pricing or accounting calculation
is changed. Email amounts use quota units divided by `QuotaPerUnit`, not an assumed
currency exchange rate. Emails contain key name and ID, never the secret key.

## Notification Templates

Both themes provide subject/body editors, placeholder insertion, an explicit
built-in Chinese notification template action and sandboxed preview. The template
is shared by all rules; rule-specific threshold/cooldown values are filled at send
time. Applying a template only edits the form. Preview uses synthetic total/period
quota examples, requires fresh root authorization, and never sends mail, writes
options or claims a cooldown. Save remains a separate action.

The option JSON now accepts `subject` and `body`. Missing or blank values fall back
to the built-in template, preserving existing configurations without a migration.
Supported scalar placeholders are `system_name`, `username`, `key_name`, `key_id`,
`scope`, `used_quota`, `quota_limit`, `quota_unit`, `usage_percent`,
`threshold_percent`, `cooldown_days`, `reset_at`, and `generated_at`, enclosed in
double braces. No secret API Key placeholder is available. Total-quota reset is
shown as not applicable; period timestamps use Beijing time.

Unknown/malformed placeholders, oversized templates and subject newlines are
rejected. Subject/body limits are 800/32768 bytes, with a 65536-byte serialized
configuration limit. All variable values are HTML escaped in the body. The sender
renders the latest persisted template before recipient pacing or a durable claim;
invalid rendered subjects do not consume the rule cooldown. The existing safe
mail renderer also bounds the final body. Custom HTML previews use an empty iframe
sandbox and a restrictive CSP, without external images or scripts.

## Performance, delivery and bounded storage

Quota operations only perform an atomic enable check and nonblocking notification.
No additional Redis, SQL, SMTP call, goroutine creation or blocking queue wait is
added to the forwarding path. Two background workers independently sample and send.

- Notifications coalesce in a bounded 4,096-entry buffer. At most 200 token rows
  are sampled each second, after a one-second delay plus the configured batch
  interval when batch accounting is enabled.
- A shared keyset sweep reads at most 200 rows every five seconds to repair dropped
  notifications and refresh inactive keys after period/configuration changes.
  Very large key inventories therefore take longer to sweep. This is eventual
  monitoring, not a zero-latency or guaranteed notification SLA.
- Both sweep and changed-ID queries filter fully unlimited keys in SQL before
  applying the 200-row limit. Finite total quotas (including legacy NULL flags)
  and unlimited-total keys with a nonempty period type and positive period limit
  remain eligible. This reduces returned rows and Redis work; without a new index
  it does not guarantee fewer physical rows examined by the database.
- Redis stores at most 10,000 usage snapshots and 10,000 queued rule candidates,
  with 48-hour idle expiry. Queue entries deduplicate by user/key/rule. Failed or
  dropped sampling is repaired by the sweep. Redis outage stops mail processing.
- Across instances, queue processing starts at most once every six seconds;
  recipient pacing permits at most one attempt per minute per email address.
  Other eligible rules wait without consuming their independent cooldowns.
- Before SMTP, the sender rereads persisted configuration, token, user and email,
  and transactionally claims a durable cooldown. Each SMTP call has a 30-second
  deadline. Already in-flight mail can finish after disabling configuration.
- Only definitely unaccepted deliveries may retry, at most three attempts with
  60/120-second backoff. Accepted mail is not resent. Unknown outcomes or interrupted
  sending remain suppressed for the cooldown, favoring duplicate prevention.
  The shared SMTP helper is reused without altering ticket/revenue sending flows.
- Two new small main-database tables hold a singleton capacity counter and at most
  10,000 rolling delivery states. One row is reused per user/key/rule, not per request
  or billing period. A transactional budget lock prevents concurrent over-capacity
  writes. Expired states are pruned in batches of 200; unexpired cooldowns are never
  evicted to make room. Capacity exhaustion delays new alerts rather than growing
  tables or bypassing cooldowns. Redis loss cannot remove durable cooldowns.

The status endpoint `/api/option/token_usage_alerts/status` requires fresh root
authorization and reports Redis connectivity, queue/cache sizes, process-local
dropped notices and delivery status counts. `sent` means SMTP accepted the message,
not proof of inbox delivery. No log-table migration, business-data cleanup or
historical consume-log scan is involved. The new small tables are migrated even
while the feature remains disabled.

## Validation

Regression tests cover quota scopes and rollover, rule independence, simultaneous
claims, Redis loss, persisted disabling, quota revalidation, retry exhaustion,
uncertain results, stale workers, capacity and queue pacing. Test SMTP is stubbed.
Model tests default to isolated SQLite. Optional `TOKEN_ALERT_TEST_MYSQL_DSN` or
`TOKEN_ALERT_TEST_POSTGRES_DSN` runs the same cases against a disposable database;
these tests drop and recreate only the alert tables, so never point them at an
application database. MySQL 8.2 and PostgreSQL 18 integration runs were verified.
