# Routing observation logs

Observation only: affinity keys, rule order, TTL renewal, retry decisions,
channel selection, error filters and response bodies retain their existing
behavior. No database migration, Redis key or frontend change is required.
This does not enable session-based routing.

## Scope and storage

Requests whose distributor model starts with `claude-` or `gpt-` are observed;
`gpt-image*` and model names longer than 256 bytes are excluded. The ordinary
text relay is instrumented, not task/job or upstream account routing.
Authentication/body-parsing failures before initialization have no observation.

- Existing consume/error rows receive `other.admin_info.routing_observation`.
  Ordinary user log views strip `admin_info`. Existing `channel_affinity` fields
  are preserved for compatibility.
- After distributor processing and the existing affinity write, one private
  application log line starts with `routing_observation` followed by JSON.
  The logger's request ID joins it to usage/error/access logs. This includes
  downstream rejection paths that have no consume row.
- Database snapshots are immutable. A success row can show
  `write_result=not_attempted` and `outcome=not_completed` because it precedes the
  actual binding write. The completion line records that later result; database
  rows are never rewritten. Do not treat an early snapshot as a final decision.
- Each observed request adds one bounded completion line and fields in existing
  rows. Account for the additional volume in private log retention/storage.

## Fields

| Field | Meaning |
| --- | --- |
| `version` | Schema version, initially 1 |
| `user_id`, `token_id`, `model`, `group` | Request identity and accounting group, no API key |
| `signals` | Source-labelled HMAC fingerprints of client correlation values |
| `lookup` | `not_evaluated`, `disabled`, `no_matching_rule`, `missing_key`, `key_pattern_mismatch`, `miss`, `hit`, or `cache_error` |
| `rule`, `binding_source`, `binding_fp` | Rule/source and fingerprint of the existing complete affinity key |
| `ttl_seconds` | Resolved rule TTL; after a write, the actual TTL passed to the cache |
| `skip_retry`, `switch_on_success`, `keep_on_disabled` | Policy at lookup; ignore if lookup was not evaluated |
| `preferred_channel` | Channel read from affinity, not necessarily the execution channel |
| `final_channel` | Channel in context at snapshot time, not proof of success |
| `events` | Ordered selection, bypass, attempt, limiter, error-filter, retry and binding-clear/preservation decisions |
| `events[].rule_index` | One-based matched error-filter index; omitted if no match |
| `write_channel`, `write_result` | Actual write target and `not_attempted`, `stored`, or `cache_error`; stored may be a same-channel TTL refresh |
| `outcome` | `not_completed`, `relay_returned` or `relay_error` |
| `http_status` | Completion response status, distinct from relay outcome |
| `dropped_events` | Events omitted after the 16-event limit |

An `attempt` records selection before the authoritative limiter, not proof that
HTTP reached the upstream. `retry_allowed` does not guarantee another attempt:
the outer retry loop can exhaust its budget. Force-retry precedence is unchanged.
`relay_returned` means no final relay API error, not a guarantee of complete SSE
output. Use existing usage/stream status too. HTTP 200 can coexist with a relay
error; this change does not alter the existing affinity write guard.

## Correlation and privacy

Header signals: `Session_id`, `Session-Id`, `Thread_id`, `Thread-Id` (case
insensitive; underscore/hyphen forms remain distinct). Already-validated DTOs
supply Responses `prompt_cache_key` and Claude string `metadata.user_id`.
Capture occurs before upstream adaptation without rereading/changing the body.

Signals use HMAC-SHA256 truncated to 128 bits, domain-separated by schema/source
and scoped to authenticated user/token IDs. The key is existing
`common.CryptoSecret`: `CRYPTO_SECRET`, otherwise the session secret or process
fallback. Cross-node/restart comparison requires consistent existing secrets;
rotation changes fingerprints. No secret/raw signal is added to diagnostics.

These are client claims, not validated conversation identities. Cache keys can
be shared across conversations and metadata may identify a user rather than a
session. Compare source-labelled signals rather than inventing a universal
session ID. Missing signals may be absent, unsupported, oversized or not yet
parsed. Changing tokens intentionally changes fingerprints.

There are at most 8 signals with inputs at most 4096 bytes. Claude metadata is
inspected only up to 8192 bytes. Binding keys above 8192 bytes have no new binding
fingerprint. Rule/source/group labels are bounded to 128 bytes. Event reasons
are local codes, not upstream messages. No prompt, authorization header, raw
session/cache key or upstream credential is added.

## Analysis after deployment

1. Compare the same signal source/fingerprint, user/token, model and group in a
   bounded time range. Consume timestamps are completion times; use access logs
   to reason about concurrent requests, not an assumed conversation order.
2. Distinguish preferred, attempted, final and written channels. A limiter detour
   may intentionally preserve the old binding (`preserve_after_rate_limit`).
3. Inspect retry-denial and error-filter events for repeated failures. Nonempty
   filter condition categories are ANDed, so status alone may not match.
4. `miss` cannot distinguish expiry, eviction and first use. No extra cache
   GET/TTL/SCAN is performed. `stored` confirms this request's write, not which
   concurrent request subsequently won.
5. Compare existing cache-read/creation tokens on successful correlated rows.
   Preserve input semantics: Claude net input needs reads and creation added to
   its denominator. Stable gateway channels do not prove stable upstream accounts.

Tests cover Redis-protocol cache operations with miniredis, immutable snapshots,
signal privacy/isolation, unchanged retry/error-filter behavior, distributor
rejection logging and user-log redaction. They make no live provider requests.
