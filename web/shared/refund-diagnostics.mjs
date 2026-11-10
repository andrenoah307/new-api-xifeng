export function refundDiagnosticLines(diagnostic, t, exact = false) {
  const reasons = {
    grace_exceeded:
      "The shortfall or wallet debt exceeds the grace limit. Manual review is required.",
    grace_unavailable:
      "The paid balance is insufficient and refund grace is disabled or invalid. Check the grace setting and recharge records.",
    unbacked_credit:
      "The shortfall is not covered by actual wallet debt. Check administrator credits, gifts and missing recharge records.",
    hold_not_eligible:
      "This refund is not fully frozen or includes gifts. Confirm the frozen amount before applying grace.",
    gift_balance:
      "Gift credit remains in this account. Confirm paid and gift balances before refunding.",
    funding_limit:
      "Original paid funds cannot cover this refund within the grace limit. Check previous refunds and recharge amounts.",
    missing_history:
      "A previous refund has no cash allocation. Reconcile the related refund before recalculating this one.",
    completed_without_snapshot:
      "This completed refund has no calculation snapshot. Reconcile its historical payment records; do not pay it again.",
    later_refund:
      "A later refund already uses this calculation. Reconcile the related refund before changing the earlier one.",
    payment_evidence:
      "Recharge evidence is incomplete or inconsistent. Check successful payments and administrator recharge records.",
    invalid_order:
      "A recharge order has an invalid amount, currency or credited balance. Check the related order.",
    allocation_mismatch:
      "Previous refund allocations do not match their recharge sources. Check historical refund amounts and linked orders.",
    payment_allocation:
      "The refund cannot be allocated consistently across recharge orders. Check rounding, previous refunds and gift recovery.",
    invoice_pending:
      "A related invoice is already awaiting correction. Finish or reconcile that correction first.",
    duplicate_invoice:
      "A recharge order is linked to multiple issued invoices. Check the related invoice and order.",
    invoice_allocation:
      "Invoice amounts or issuer allocations do not reconcile. Check the original invoice and previous corrections.",
    calculation_failed:
      "The calculation could not be completed. Retry; if it persists, check the service error log.",
  };
  const lines = [];
  if (diagnostic.amounts) {
    const amounts = Object.fromEntries(
      Object.entries(diagnostic.amounts).map(([key, value]) => {
        const number = Number(value);
        const limit = Number(diagnostic.amounts.limit);
        const hidesExceededLimit =
          (key === "shortfall" || key === "wallet") &&
          Math.abs(number) > limit &&
          Math.abs(number).toFixed(2) === limit.toFixed(2);
        if (exact || hidesExceededLimit || (number !== 0 && Math.abs(number) < 0.005)) {
          return [key, value];
        }
        return [key, number.toFixed(2)];
      }),
    );
    lines.push(
      t(
        "Verified recharge credit {{recharged}}; consumed credit and fees {{consumed}}; previously refunded credit {{refunded}}.",
        amounts,
      ),
    );
    lines.push(
      t(
        "Available paid credit {{available}}; requested credit {{requested}}; shortfall {{shortfall}}.",
        amounts,
      ),
    );
    lines.push(t("Wallet balance {{wallet}}; grace limit CNY {{limit}}.", amounts));
  }
  lines.push(t(reasons[diagnostic.code] || reasons.calculation_failed));
  return lines;
}
