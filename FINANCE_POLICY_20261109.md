# Invoice and Refund Policy

## Payment and Invoices

As of 2026-11-06, both themes expose administrator settings for fee collection,
ordinary/special fee percentages (0 through 99.99%), calculation mode, historical
eligibility cutoff, and free ordinary invoices for qualifying historical payments.
Saving clears the legacy `policy_effective_at` schedule to zero. Custom settings
apply to new checkout quotes immediately; reads no longer overwrite those saved
settings with the November 9 schedule. Module visibility and fee collection remain
separate switches. Both invoice types still have **6% VAT**, stored and exported
independently of the configurable invoice fee.

The cutoff uses **00:00 Asia/Shanghai**, excluding that date. Eligibility follows
successful payment completion, not order creation; unknown completion times do
not qualify. A blank cutoff stores zero and disables historical eligibility.
With `legacy_free_ordinary_enabled` on, qualifying CNY payments, including prior
no-invoice payments, can obtain ordinary invoices without an additional fee.
This exemption does not grant special invoice eligibility. Turning it off restores
the existing eligibility and fee rules; it does not make every no-invoice order
eligible. Previously paid fees are not automatically refunded by this switch.

Explicit order invoice types take priority over the historical exemption:
ordinary orders are eligible only for ordinary invoices, and special orders only
for special invoices, even before the cutoff. Their prepaid net/fee snapshots
remain authoritative. The historical free-ordinary fallback applies only to
orders with an empty invoice type or `none`. Both the eligible-order list and
the quote/create path enforce this rule, including rejection of mixed types.
The filter regression covers both exemption switch states, prepaid snapshots and
actual special-invoice creation. Model/controller package tests, invoice/manual
top-up race tests and model/controller vet pass. Both themes already send the
selected invoice type and need no frontend changes for this fix.

Compatibility boundary: until an administrator saves, existing scheduled configs
(including old JSON missing the schedule field) retain the former November 9,
2026 Beijing midnight schedule (`1794153600`), 6%/9% fees, and historical exemption.
The settings controls are editable even in this compatibility state. A save
explicitly replaces that schedule; it is not a new configurable future fee start.

Checkout stores fee, net credit, price, quota unit and discount snapshots. New
custom checkout orders preserve their quoted amounts if settings later change.
Already pending orders carrying the old schedule still settle under their stored
schedule and completion time, preserving their original quota-per-net-cent ratio.
Completed payments are idempotent and never repriced on callback replay. Fees
round to cents first; credited cash is the gross payment minus that fee.
This configuration change adds no database columns and does not alter refunds.

## Manual Top-Ups

Manual top-ups require a configured corporate receipt account only when ordinary
or special invoices are selected. The historical `-` choice (API value `none`)
allows an empty account, including when no corporate accounts are configured.
If an account is selected, it must still be valid and its receipt metadata is
snapshotted. Quote tokens and request IDs still protect confirmation and replay;
omitting an account does not grant invoice eligibility or change fee calculation.
Regression checks cover missing configuration, invalid accounts, required invoice
accounts, and idempotent account-free receipts. Both themes pass build and lint;
Default also passes typecheck. Manual top-up race tests and model/controller vet
pass. This update does not add schema changes or deploy to production.

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

Refund reconciliation details now open in a dialog in both themes. Previously,
Details only selected a panel below the entire paginated list, so clicking it
appeared to do nothing (especially when the first row was already selected).
No row is selected automatically; the dialog identifies the selected ticket,
supports closing and switching records, and constrains wide tables on mobile.
Classic refreshes retain the selected detail while the request is in flight.
Existing finance role checks and settlement/owner confirmation rules are unchanged.
Opening the dialog does not initiate any refund or settlement mutation.
Both theme builds, Default typecheck, changed-file lint and Playwright checks at
1440px and 390px pass (open/close, switching tickets, waiting-state guard and no
mutation on inspection). Wide tables scroll within the dialog content.

Issued invoices do not expose Reject Application in the invoice UI. For refunds
with generated correction records, finance staff use Refund reconciliation >
Details > Invoice correction, start processing, then record external red invoice
and (where required) reissued invoice references and evidence. Resolving the
original invoice ticket is not completion of a refund correction. The legacy
invoice-history warning in Default still reflects original invoice status only;
the correction record is authoritative for whether correction work is complete.

Invoice CSV exports in both themes now contain nine columns: email, quantity,
unit price, total amount, company details, taxable service name, invoice type,
VAT rate, and issuer (corporate account name). Ticket ID, fee calculation mode, supplementary payment amount,
and payment routing are omitted from the CSV. Backend records and API fields
remain available for existing finance workflows. Output checks cover aligned
headers/rows, empty exports, quoted multiline company details and formula safety.

Each ticket expands into one CSV row per corporate account name, aggregating
payments with the same name using integer cents. For example, a CNY 41 invoice
with CNY 11 paid to A and CNY 30 to B exports two rows, 11/A and 30/B.
Names come only from immutable payment routing snapshots, never current gateway
settings. Version 1 payments use their gross payment cents (including prepaid
invoice fees); legacy payments use their recorded money. Pagination and selection
still operate on tickets. The API adds computed `issuer_amounts`; no migration
or financial data mutation is involved. Both themes must ship with the backend.

Unknown/private accounts, missing orders and unallocated supplementary amounts
are grouped under the literal `-`. Orders owned by another user are never read
into the breakdown. Duplicate order IDs are counted once. When original order
amounts exceed the ticket total (for example after refunds), the whole ticket is
exported under `-`: there is no reliable per-order invoice allocation snapshot,
so amounts must not be prorated or assigned to an arbitrary issuer. Every ticket's
exported rows sum to its recorded invoice total. Legacy API responses without a
breakdown also produce a single `-` row. CSV formula protection remains active
for account names; the standalone placeholder `-` is emitted literally.

The export keyword filter now passes a grouped GORM expression to `Where`,
instead of a function value that caused `model value required` on searches.
The integration regression exercises keyword filtering with the issuer breakdown
and verifies that a two-row export still counts as one paginated ticket.
Validation: model/controller package tests and vet, issuer export race regression,
both theme builds, Default typecheck, changed-file lint, and direct CSV output
checks pass. No production deployment or database writes were performed.

The configurable-policy update passes model/controller/middleware/router package
tests, service finance tests, model/controller finance race tests, and vet across
those five packages. New regressions cover custom values surviving saves, free
ordinary eligibility and quotes, blank and exact cutoff boundaries, fee disable,
and pending checkout snapshots across settings changes and callback replay.
Both themes build, Default typecheck passes, and modified frontend files pass
lint (Classic excludes its obsolete copyright-header rule). Desktop/mobile browser
checks cover editing legacy configurations, custom fee percentages, Beijing dates,
free/fee switches, saved-value reload, and clearing the cutoff. No production
deployment or live payment was performed for this update.

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

## Cooling and Audit Baseline: 2026-11-06

All 33 finance commits from `172440506..389a0bddb` were replayed onto
`origin/dev-20260712` at `bcd7722ca`. The new base includes transactional pressure
cooling recovery, administrator-only channel automation logs, per-channel
consecutive cooldown limits and suspension indicators in both themes. See
`PRESSURE_COOLING_RECOVERY.md` for behavior and operational limits.

Conflicts were confined to locale files and resolved by JSON key. All 15 locales
preserve the finance values plus the four new cooling messages. Classic key order
is retained to avoid unrelated churn. Outside locales, the resulting change from
the original finance tip has exactly the same stable patch ID as the two new
development commits; finance policy and refund settlement behavior are preserved.

Validation: service/model/controller/middleware/router packages passed with an
isolated ClickHouse instance; both frontend builds and Default typecheck passed.
Targeted finance/refund/invoice/cooling race tests, core Go vet and 39 frontend
cooling regression tests also passed.
This update does not deploy either branch or run production migrations.
