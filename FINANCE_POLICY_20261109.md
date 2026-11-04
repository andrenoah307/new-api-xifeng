# Invoice and Refund Policy: 2026-11-09

## Payment and Invoices

The policy boundary is **2026-11-09 00:00:00 Asia/Shanghai**, Unix timestamp
`1794153600`. Eligibility follows successful payment completion, not order creation.
Ordinary invoices deduct 6% of the gross payment; special invoices deduct 9%.
Both invoice types have **6% VAT**, stored and exported independently of the fee.
Fees round to cents first; credited cash is the gross payment minus that fee.

The schedule is applied automatically, including to existing configuration JSON
without the schedule field. The settings API enforces the published schedule.
The finance module visibility switch does not disable the payment policy.
Pre-boundary CNY payments remain eligible for free ordinary invoices, including
orders previously marked as no-invoice. They do not qualify for free special invoices.
Post-boundary no-invoice orders cannot acquire invoice eligibility retrospectively.

Checkout stores the price, quota unit and discount. Payment completion applies
the policy for its completion time using these snapshots. Pending orders created
by an earlier version preserve their original quota-per-net-cent ratio. Completed
payments are idempotent and are never repriced on callback replay.

## Refund Workflow

1. The owner submits a refund ticket with a stable UUID request key. This creates
   a pending request without debiting the wallet and suspends new relay admission
   for that user, including across application instances.
2. Existing requests and asynchronous tasks finish settlement. The owner uses
   Refresh in the ticket to request a quote. An unverified liability produces no
   payable quote. Ledger mismatch requires financial reconciliation.
3. The owner confirms the displayed amount. The server repeats the quote under
   the same user-row lock used by request admission, checks its token, freezes
   the quota and reserves order allocations atomically. Duplicate confirmation
   with the same amount/token cannot freeze twice. Staff cannot confirm for the owner.
4. Finance staff claim each invoice correction before external work, reverse its
   active invoice, and record references/evidence for the replacement. Once a
   correction is processing, cancellation/rejection cannot undo that external work.
5. An administrator confirms completion only after corrections are complete and
   settlement remains verified. Cash payout remains the existing **manual** finance
   operation; this change does not call a payment-provider refund API.

Cash refund = original paid cents * refunded quota / original credited quota.
Cumulative rounding conserves cents across repeated partial refunds.
Replacement invoices cover actual consumption **including the retained fee**.
For paid 100, credited 94, consumed 47: refund 50 and reissue 50. If only half of
the unused quota is refunded, refund 25 but still reissue 50, not 75.
Zero consumption produces no replacement invoice. Merged invoices retain the
coverage of unaffected orders. Later corrections use each affected order's last
invoiced consumption; a previously zero replacement requires no fictitious red
invoice reference if subsequent consumption needs a new invoice.

New owner endpoint: `POST /api/ticket/refund/finance-confirm`, with `ticket_id`,
`refund_quota` and `quote_token`. Existing finance-quote GET returns a payable
token only for a pending, unconfirmed request whose liabilities have settled.

## Settlement Safety and Deployment

`refund_liabilities` is in the shared primary database. Admission locks the user,
checks pending refunds, then registers before forwarding. Records include the
request ID and node name for reconciliation. A registration/database
failure rejects the request. Each record needs both request completion and any
asynchronous settlement completion before deletion. The order of these events
does not matter. Billing failures, panics, task insertion/writeback failures or
process loss retain the record. **There is no expiry or administrator bypass.**
Failed task refunds also reverse their wallet-consumption aggregate; queued
aggregate updates must drain before the ledger can reconcile.

Deploy the backend and both frontend themes together. **Do not run mixed old and
new relay instances while accepting executable refunds.** Before enabling traffic
on the upgraded fleet, stop admission on all old instances, drain requests and
async tasks, verify their settlement, flush queued accounting writes, then migrate
and start the upgraded instances. Existing versions have no durable registration,
so an empty new table alone cannot prove that old traffic settled. This is a
deployment prerequisite, not something the migration can infer automatically.

Back up the database before migration. Both migration paths add the liability
table, confirmation fields, VAT rate and checkout snapshots. Use shared primary
DB access on every node; Redis availability is not settlement evidence. Admission
adds a short user-row transaction per request and must be capacity-planned.

For retained records, reconcile the request/task, wallet changes, queued usage and
provider result before any repair. Do not delete records by age, restart, task
terminal status alone, aggregate equality alone or an assumed timeout. A terminal
task can precede its financial write. There is deliberately no force-refund button.
Historical orders without exact payment/quota snapshots, unsupported currencies,
unallocated old fees, or inconsistent mixed-source balances stay in reconciliation;
the system does not invent missing accounting history.

## Verification

Regression coverage includes ordinary/special payment boundaries, callbacks across
the cutoff and replay, historical free invoices, independent VAT exports, full and
partial refunds, owner confirmation, duplicate/concurrent confirmation, admission
versus refund races, invoice correction barriers, failed settlement and panic.
The refund flow is exercised on SQLite, MySQL 8.2 and PostgreSQL 18. These runs do
not claim execution on the minimum supported MySQL 5.7/PostgreSQL 9.6 versions.
Both themes have build/type checks and desktop/mobile browser interaction checks.

The full `go test ./...` run also exposes the pre-existing
`pkg/requestip.TestGetClientIPDefaultsToRemoteAddr` failure: its expected remote
address disagrees with the existing proxy-header fallback. This unrelated module
is unchanged. Classic's two refund components also have existing license-header
lint mismatches (missing header / 2023-2026 versus the linter's hard-coded 2025);
functional lint passes with only that rule excluded, preserving existing attribution.

## Rebase onto the Development Branch (2026-11-04)

All 32 finance commits from `04e21bde3..2e938cb41` were replayed onto
`origin/dev-20260712` at `915daf41e`. The policy and durable refund settlement
logic are preserved. Channel revenue reports, scheduled summaries, API key alerts,
captcha protection and Docker shared frontend assets from the new base are retained.

Conflict resolution preserves both captcha and finance route registration, both
dashboard destinations, and both sets of CI finance tests. Fixed-amount checkout
quote/payment routes now also enforce the discount-code captcha when a code is
present, after the finance module gate. Both themes already use the shared captcha
interceptor. A direct-route regression test covers these additional endpoints.
All 15 locale files retain every key from both original branch tips.

Two integration checks required small compatibility fixes: narrow the CSV response
Content-Type to a string before checking it, and import Axios by package name in
the shared captcha test so Bun workspace dependency hoisting is supported.

Validation after rebase: core model/service/controller/middleware/router tests,
targeted finance/refund/invoice/captcha race tests, Default typecheck/build, Classic
build, core Go vet, targeted frontend lint, and six frontend captcha/refund tests passed.
The full Go suite is not green: the existing request-IP test above still fails,
and `pkg/realtime_metrics.TestChannelHourlyCache` failed in the full run but passed
when rerun alone on both finance and the original development base. Neither package
was changed by this rebase. SQL dialect integration
and browser checks described above were performed before this rebase, not repeated.

Rebase pitfalls: merging routes must retain authentication, module gates and captcha
checks together; locale conflicts require key-level merging; CI conflict resolution
must retain channel-report tests as well as refund tests. Wait for frontend builds
to finish before full Go tests because the root package embeds their generated assets.
The development branch is unchanged. This is a source update, not a deployment.
