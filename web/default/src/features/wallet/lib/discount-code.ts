export function exceedsDiscountCodeLimit(
  amount: number,
  maxAmount = 0
): boolean {
  return maxAmount > 0 && amount > maxAmount
}
