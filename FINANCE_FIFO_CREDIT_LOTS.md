# FIFO Credit Lots and Proportional Gift Recovery

## Accounting contract

Each successful payment owns an immutable cash receipt C (integer cents), paid
credit P (integer quota), and attributed gift B (integer quota). C is never
reconstructed from today's exchange rate. Checkout base quota is paid credit;
the separately persisted discount bonus is a gift, not cash. Reviewed historical
evidence may split an embedded gift out of the old total. A discount percentage
alone is insufficient to reinterpret an ambiguous historical receipt.

Payment lots are ordered by completion time, then ID. Settled usage consumes
the paid credits of ALL lots first in FIFO order, then their gifted credits in
FIFO order. Gifts from an earlier payment never displace paid credit from a
later payment in this accounting projection.
Unattributed noncash grants remain nonrefundable and cannot silently be assigned
to a payment. Existing aggregate wallet accounting remains the relay source of
truth; a deterministic lot projection attributes settled usage without adding
database writes to every streamed token. Quotation still requires a balanced
ledger and no unsettled liabilities, under the wallet lock at confirmation.

## Refund calculation

For paid credit q selected from a lot, cash is the difference between rounded
cumulative C * refundedPaid / P and cash already refunded. Service fee recovery
uses the corresponding cumulative fraction of original cash. Gift recovery is
the difference between rounded cumulative B * refundedCash / C and gift already
recovered. In addition, a refund reaching a later lot expires all remaining
gifts of earlier lots. These are explicit gift-only allocation rows, with zero
cash and no invoice correction. Gift credit never generates cash.

Example: C=9000 cents, P=90, B=10, usage=45. Refund all remaining paid credit:
cash=4500 cents, paid removal=45, gift removal=5, remaining wallet=5 gifted credit.
No usage: refund 9000 cents and remove 90 paid plus 10 gift. Usage=95: remaining
paid credit is zero; the remaining 5 gifted credits cannot be refunded.
Two lots each C=9000, P=90, B=10, usage=135: the first paid balance is exhausted,
the second retains 45. Refunding 4500 cents removes 45 paid quota, all 10 gifts
from the first lot and 5 from the second. The remaining wallet is 5 gifted quota.
Usage=90 with a single lot leaves all 10 gifts usable, but no refundable cash;
no refund means no automatic gift expiration.

Partial refunds use cumulative integer rounding to prevent repeated rounding
from creating cash or gifts. A gift cannot be recovered twice or below zero.
If prior data conflicts with the original lot, quotation must require review,
not silently debit another payment's gift or change an existing refund receipt.

## Lifecycle

Quote -> owner confirmation -> reserve paid and gift credits together -> invoice
correction -> payout -> commit both removals. The ticket records refundable paid
quota separately from recovered gift quota; frozen quota is their sum. Rejecting
or cancelling restores the full frozen amount exactly once. Unknown payout
results preserve the existing hold. Cash, fees and commissions still follow the
actual payment fraction. Invoice correction uses cash, never gift face value.

Existing confirmed refunds retain their recorded allocations (zero gift recovery
for old rows). New quotes include a calculation policy version. Changing a lot
or its reviewed split invalidates an unconfirmed quote. The existing opt-in
invoice-attribute split remains a restriction on refund selection after global
FIFO consumption; it must never regroup usage.

## Delivery and validation

Additive bigint fields record original gift and recovery on allocations, and
total gift recovery on tickets. Existing NULL/new fields must read as zero on
SQLite, MySQL and PostgreSQL; no payment receipt or wallet is rewritten by
migration. Deploy both themes and backend together after backing up the database;
after new gift-bearing refunds exist, do not roll back to the old binary.

Tests cover the 90/100/45 and two-payment/135 examples, partial and repeated
refunds, all paid balances before gifts, prior-gift expiry, cancellation,
existing confirmed refunds, invoice splitting,
rounding conservation, stale quotes and cash-only invoice/commission accounting.

Validated: model/controller suites, targeted race tests, vet, both theme builds
and Default type checking/lint. Disposable MySQL 8.2 and PostgreSQL 17 databases
exercise gift holds, expiry, payouts, rounding and upgrading old rows; SQLite
exercises the same upgrade contract. Eight desktop/mobile browser cases cover
quote disclosure, paid-only confirmation input and confirmed gift audit rows.
Classic loads `zh-CN`/`zh-TW`, so Chinese translations must reach those files;
updating its legacy `zh.json` alone does not update the running UI.
