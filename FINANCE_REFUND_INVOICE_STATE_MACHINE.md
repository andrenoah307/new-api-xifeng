# Refund Reconciliation and Invoice Correction

Implemented on `finance-dev-20261021`. Test-site investigation was read-only;
deploying code does not approve or repair historical financial records.

## Actual Payment

- The current FIFO/gift policy is specified in [FINANCE_FIFO_CREDIT_LOTS.md](FINANCE_FIFO_CREDIT_LOTS.md).
  All payment lots' paid credits are consumed before any gifted credits.
  Refunds recover a proportional gift from the final refunded lot and expire
  all remaining gifts of earlier lots. Gift-only allocation rows carry zero
  cash, and therefore no invoice correction or commission clawback.
- `refund_quota` remains paid credit; `refund_gift_quota` records noncash recovery;
  `frozen_quota` holds their sum. Cancel/reject restores both, while successful
  payout commits both. Previously confirmed refunds keep their original zero
  gift recovery. New quotes use calculation version 2. Known gift-only balances
  cannot create a waiting refund; gifts remain usable when no refund occurs.
- Both themes and CSV export show cash, recovered gifts, remaining paid/gift
  value and recovery reason separately. Gift value uses the original payment's
  net-cash-to-paid-quota ratio for display, never today's exchange rate, and is
  not subtracted again from cash. Old rows without this snapshot display `-`.
- This change adds nullable bigint/snapshot columns to refund allocations and
  tickets. Back up before upgrading and restart the backend with matching web
  assets; after gift-bearing refunds exist, do not roll back to a binary that
  assumes frozen quota equals only paid refund quota.

- Optional `InvoiceTaxConfig.separate_refund_invoice_types` defaults to false,
  including existing option JSON that omits it. Administrators can save it in
  payment gateway invoice settings in either theme. Off preserves mixed refunds.
  On separates empty/`none` invoice types from `ordinary`/`special` types using
  the original order attribute, not current invoice issuance or free-fee policy.
- Consumption remains global FIFO (`complete_time`, then order ID). A quote
  stops at the first remaining order of the other class, even if later orders
  match the first class. The owner sees the class and actual cash amount and
  confirms only the allocated quota. After completing that refund, a new refund
  processes the remaining orders. No automatic extra tickets or payouts occur.
- The class is part of the quote hash. Changing this option invalidates an
  unconfirmed quote; confirmation also checks allocated quota equals the request,
  preventing freezing the original mixed amount against a partial quote.
  Already confirmed allocations remain valid and are not recalculated.
- This split is not a historical ledger repair. Missing payment snapshots,
  unexplained balances and unsettled liabilities still require existing review.
  Actual cash and collected fee refunds retain original payment proportions and
  cumulative cent rounding. Current display exchange rates do not price refunds.
  No schema migration is added; deploy backend and both frontends together.
- Verified with model/controller tests, both frontend builds and Default type
  checking, changed-file lint, and 12 desktop/mobile browser cases across both
  themes (option off and both order classes). Browser assertions check the
  confirmation submits only the quoted allocation total.

- Reconciliation submission is not approval. Under the user's wallet lock, an
  identical pending proposal by the same actor/source revision returns its
  existing record even with a new request key; a different proposal is blocked
  until independent review. Approving one legacy duplicate supersedes other
  pending records for that user/source revision, preserving their audit history.
- Reconciliation records are internal audits, not public ticket replies. New
  submissions/reviews no longer write public messages; the user ticket endpoint
  filters legacy staff messages beginning with `Financial reconciliation #`.
  Admin ticket views retain old audit messages. No database migration is needed.
- Pending accounting review and ready-for-owner-confirmation now have distinct
  refund status text. Approval only applies evidence; remaining ledger problems
  still block a quote. After reconciliation and liabilities are clear, the owner
  refreshes and confirms the quote; invoice correction and payout then proceed.
- Read-only investigation found test refund 11 proposals 1 and 2 identical,
  both submitted by actor 3 and neither reviewed. No test-site approval or
  account correction was performed. Another administrator must review a
  proposal; its zero additional grant does not resolve the separate difference.

- Service-fee and included-bonus inputs share aligned rows; explanations sit
  below the paired inputs. Empty adjustment inputs explicitly mean zero;
  clearing the bonus input no longer falsely displays zero purchased balance.
  Invalid numeric entries remain invalid, rather than silently becoming cash.
- Included bonus is a documented gift within that recharge; additional gifts
  are separate grants outside recorded recharge orders. Neither is computed by
  subtracting original cash payment from today's converted balance. The latter
  mixes historical payment with a current display valuation and is not a gift.
- A read-only follow-up of test refund 11 found reconciliation proposal 1 already
  submitted by user 3: paid 59940 cents, fee 0, purchased quota 333000000,
  included bonus 0, additional grant 0. It awaits independent review; the agent
  did not submit or approve it. The separate ledger mismatch remains unresolved.

- The reconciliation field is labelled "Collected invoice service fee (CNY)":
  historical invoice service charges only, never gateway gross payment or usage.
  Free ordinary-invoice eligibility concerns issuing an invoice; it cannot prove
  the provenance of a refund balance or erase fees already collected.
- Read-only test-site verification confirmed legacy free ordinary invoices are
  enabled before 2026-11-09 00:00 Beijing time (1794153600), with fee collection
  currently disabled. Order 9 has no original invoice type and was completed
  before the cutoff, so it qualifies for that invoice policy. Refund 11 remains
  blocked by missing payment allocation history and a ledger mismatch, not an
  unpaid invoice service fee. Orders 11 and 12 retain historical fee snapshots
  of CNY 6.66 and CNY 6.00; changing current policy does not rewrite these.

- Finance's normal refund view is read-only for calculation inputs: it shows
  consumed actual payment, cash refund, remaining invoice amount, full invoice
  reversal and reissue amounts returned by the existing allocation/correction
  engine. Raw quota columns are removed from this view. Historical repair forms
  are collapsed behind an administrator-only UI entry; an unresolved history
  displays a pending administrator review state, never fabricated zero amounts.
- Consumption allocation remains FIFO across paid orders. For each order,
  consumption and refund quota map proportionally onto its original paid cents;
  current quota units/exchange rates cannot reprice historical cash. Separate
  bonus records are noncash. Invoice reversal covers the entire active invoice;
  reissue equals active invoice minus refund cash, retaining untouched orders.
- Regression example: a discounted CNY 90 order has 40% consumed and 60%
  refunded: CNY 36 consumed, CNY 54 refunded. A combined CNY 150 invoice with an
  untouched CNY 60 order is reversed for CNY 150 and reissued for CNY 96. A
  CNY 5.40 collected fee allocates CNY 3.24 to the refund. Separate gift credit
  and changes to present-day exchange settings do not add refundable cash.

- Reconciliation evidence forms show original actual payment and total credited
  value read-only. Finance enters collected fees and documented included bonuses;
  purchased quota is derived as original credited quota minus included bonus.
  This identity is calculated in integer quota, not rounded display money.
- Source evidence is an optional note with no field-specific minimum or maximum
  length in either theme or the submission API. Empty notes do not prevent
  submission or independent review. Amount and role checks still apply.
- Submission validates on click and explains invalid fees/bonuses and excessive additional gifts.
  Unexplained differences remain pending unless supported by real records;
  discount rate alone does not prove an included or additional bonus.
- Read-only test-site verification of refund 11 confirmed legacy payment 9:
  actual CNY 599.40, 333,000,000 credited quota, rate 90, no financial snapshot.
  Current settings are 500,000 quota/USD and CNY 7.3/USD: credited value CNY
  4,861.80 and unexplained difference CNY 321.20. These are credit valuations,
  not the cash refund amount. No test-site evidence or approvals were written.

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
