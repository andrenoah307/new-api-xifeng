# Refund Reconciliation and Invoice Correction

Implemented on `finance-dev-20261021`. Test-site investigation was read-only;
deploying code does not approve or repair historical financial records.

## Actual Payment

- Reconciliation summaries, evidence inputs and review history show CNY instead
  of raw quota. Credit value is quota / system QuotaPerUnit * USDExchangeRate,
  labelled as converted value; it must never replace the original paid cash
  used for refunds. Actual cash and collected fee inputs are in yuan.
- The report supplies current conversion settings, independently of personal
  currency preferences. Rational conversion preserves untouched quota inputs;
  the API continues receiving integer quota and cash cents. Invalid settings
  disable evidence submission instead of assuming a rate.
- Conversion settings participate in the source revision. After this upgrade
  or a rate change, stale proposals/quotes require refresh and resubmission.
  Deploy backend and both themes together; this display change has no migration.

- Actual payment is authoritative: checkout v1 uses `payment_total_cents`;
  legacy evidence must match original `money` converted to integer cents.
- Cash refund = original paid cents * cumulative refunded purchased quota /
  original purchased quota, rounded, minus previously refunded cents.
  Existing FIFO consumption allocation remains in force. Never use today's
  exchange rate, list price, invoice net amount or discount rate to reconstruct cash.
- Collected fee and commission deductions use cumulative refunded cash /
  original actual payment, minus previously allocated rounded portions.
  Repeated partial refunds conserve original cents and commission.
- New recharge commissions use actual payment, including prepaid invoice fees.
  Historical grants are not rewritten. Optional refund clawback uses original
  per-order commission, capped by available inviter commission and history.
  Legacy refunds without cash allocations no longer receive a suggested clawback
  derived from quota at the current exchange rate.
- Separate discount_bonus records are noncash. Legacy embedded bonuses require
  documented classification: purchased plus embedded bonus equals original quota.
  No original payment, balance or recorded consumption is rewritten.
- Remaining invoice amount = active invoice amount minus actual refunded cash,
  not actual consumption alone. Fully reversed invoices need no second reversal.
  A nonzero active amount below the proposed reduction requires review.

## Read-Only Test-Site Findings

Refund ticket 11 / user 9 was pending and unverified, with no frozen quota,
allocations or corrections. Invoice ticket 10 was issued for CNY 54; its resolved
ticket status did not establish completed red-invoice work.

Legacy payment 9 recorded CNY 599.40, discount rate 90 and quota 333,000,000,
without a financial snapshot. Successful paid sources totalled 672,170,000 quota;
wallet plus recorded consumption totalled 694,170,000, a 22,000,000 difference.
The complete recharge list had no separate successful bonus row for this user.
Available logs did not establish the difference's source. This is not proof of
cash loss or authorization to classify the difference as a gift. Obtain historical
receipts and grant evidence before proposing a reconciliation.

## Refund State Machine

| State | Action | Guard / result |
| --- | --- | --- |
| Waiting for settlement | Owner refreshes; staff inspects evidence | Durable liabilities must clear; consumption remains suspended |
| Needs reconciliation | Finance/admin submits evidence | Proposal only, no wallet mutation |
| Proposal submitted | Another admin/root approves or rejects | Fresh source revision, valid evidence, different reviewer |
| Awaiting owner confirmation | Owner refreshes quote and confirms | Recheck ledger/liabilities, freeze quota, create allocations/corrections atomically |
| Corrections pending | Finance/admin handles invoices | Payout stays blocked |
| Ready for payout | Admin/root claims manual payout | One unresolved attempt per ticket |
| Payout processing | Register actual receipt, unknown outcome, or proof of no payment | Verified receipt completes refund transaction |
| Outcome unknown | Verify external result | No duplicate attempt or release of frozen funds |
| Attempt failed | Retry with new key or cancel/reject if invoice guards permit | Evidence that no payment occurred required |
| Refunded | Read audit or replay exact request | No second deduction or claim |
| Cancelled/rejected | Read audit | Return only genuinely frozen quota |

External payment is manual: claiming an attempt sends no money. Unknown outcomes
must stay unknown until the bank/payment-system result is verified. Review never
replaces owner confirmation. Legacy v0 refunds retain their existing protocol;
new v1 completions require a claimed attempt and actual payment evidence.

## Reconciliation

`finance_reconciliations` records portable TEXT JSON plans, evidence, source
revision, request key, proposer/reviewer, outcome and timestamps. Approved plans
overlay the effective source ledger; original records remain untouched.

Supported plans supply missing legacy paid/fee snapshots, classify embedded
bonuses and identify evidenced additional noncash grants. Additional grants cannot
exceed the positive unexplained gap. Plans cannot change actual cash or already
versioned payments. A different enabled admin/root must review. Changed source,
wallet, liabilities or approved evidence invalidates approval.

Diagnostics report missing snapshots, unsupported currency, unallocated legacy
refunds, liabilities and ledger differences. Unsupported currencies, missing
historical refund allocations and unallocated historic fee payments remain
blockers. There is no force-complete action or invented cash allocation.
Resolving a supported payment/bonus gap does not waive other accounting checks.

## Invoice Correction

Both invoice and refund details display the same correction records. The original
invoice remains issued, with subsequent legal-document history recorded separately.
Issued invoices cannot be changed to rejected.

| Phase | Action | Required result |
| --- | --- | --- |
| documents_required | Save original evidence | Each issuer, real original number, original cents and reduction cents; totals reconcile |
| ready | Start correction | Claim external work; linked refund verified and liabilities clear |
| red_processing | Register red invoices | Real red references for every document and evidence |
| reissue_pending | Register replacement invoices | References/evidence for every remaining amount |
| completed | Read history / exact evidence replay | All required red and replacement documents recorded |
| cancelled | Read history | Independent, unstarted corrections only |

A replacement failure stays reissue_pending. Uncertainty while reversing stays
red_processing. Neither allows payout or release of frozen funds. Refresh and
verify external results after a lost response; do not repeat external work blindly.

Issued invoice details offer independent full reversal or same-amount replacement,
only without an active correction or pending owner refund. This never refunds cash
or alters balances. Later corrections must reference preceding replacement
documents and match issuer and amount.

Multiple issuers are recorded as structured documents in TEXT JSON. Aggregate
references are display-only. Initial originals require staff evidence because old
tickets lack legal invoice-number snapshots; a ticket ID is not an invoice number.
Version-0 corrections retain their completion endpoint; invoice details link to
the refund ticket to finish them.

## Payout, Permissions and Audit

`refund_payout_attempts` stores cash, state, request key, ticket/owner, claimant,
operator, timestamps, receipt and evidence. A unique receipt digest prevents the
same receipt closing multiple refunds. Prefix receipt references with the payment
system/account when numbers are unique only within that system. Exact retries do
not duplicate deductions. Optional commission recovery is calculated server-side.

Finance users reconcile evidence and invoices; admin/root approves another
person's proposal and confirms payout. Every endpoint checks the fresh enabled
database role. Wallet locks and guarded updates serialize transitions. Audit
messages are transactional and visible to the ticket owner: do not enter credentials,
unrelated customer data or private banking secrets as evidence.

## Migration and Deployment

Adds two tables and nullable request-key/phase/document fields to correction
records. No original payment or wallet rewrite. Removes the old unique
(refund_ticket_id, invoice_ticket_id) index to permit multiple standalone
corrections with refund ID zero. New keys, owner locks and active-operation guards
protect new writes. Existing v0 records retain their completion protocol.

Back up the database, stop old writers, and deploy backend and both themes together.
Old code does not understand standalone corrections or payout claims. Downgrading
after new records exist requires compatibility review, not just reverting a binary.
MySQL/PostgreSQL use row locks; SQLite uses transactional writes and guarded
updates. Retry transient locking failures with the same request identity.

No test-site or production deployment is included.
