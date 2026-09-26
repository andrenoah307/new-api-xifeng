export function formatDiscountCodePendingDuration(seconds, t) {
  if (
    typeof seconds !== 'number' ||
    !Number.isSafeInteger(seconds) ||
    seconds <= 0
  ) {
    return null;
  }
  if (seconds % 3600 === 0)
    return t('{{count}} hour', { count: seconds / 3600 });
  if (seconds % 60 === 0) return t('{{count}} minute', { count: seconds / 60 });
  return t('{{count}} second', { count: seconds });
}
