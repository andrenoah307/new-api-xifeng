export function formatDiscountCodePendingDuration(
  seconds: unknown,
  t: (key: string, options: { count: number }) => string
): string | null {
  if (
    typeof seconds !== 'number' ||
    !Number.isSafeInteger(seconds) ||
    seconds <= 0
  ) {
    return null
  }
  if (seconds % 3600 === 0) {
    return t('{{count}} hour', { count: seconds / 3600 })
  }
  if (seconds % 60 === 0) return t('{{count}} minute', { count: seconds / 60 })
  return t('{{count}} second', { count: seconds })
}
