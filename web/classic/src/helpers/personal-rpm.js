export const PERSONAL_RPM_REFRESH_INTERVAL = 15000;

export function normalizePersonalRPMItems(value) {
  if (!Array.isArray(value)) return [];
  return value
    .filter(
      (item) =>
        item &&
        typeof item.model === 'string' &&
        item.model.length > 0 &&
        Number.isFinite(item.rpm) &&
        item.rpm > 0,
    )
    .slice()
    .sort((a, b) => {
      if (a.rpm !== b.rpm) return b.rpm - a.rpm;
      return a.model < b.model ? -1 : a.model > b.model ? 1 : 0;
    });
}

export function personalRPMDisplayState(status, items) {
  if (status === 'unavailable' || status === 'overflow') return 'unavailable';
  if (status === 'empty' || items.length === 0) return 'empty';
  return 'available';
}

export function installVisibleTopRefresh({
  documentRef,
  windowRef,
  refresh,
}) {
  const tick = () => {
    if (documentRef.visibilityState === 'visible') refresh();
  };
  const interval = windowRef.setInterval(tick, PERSONAL_RPM_REFRESH_INTERVAL);
  documentRef.addEventListener('visibilitychange', tick);
  return () => {
    windowRef.clearInterval(interval);
    documentRef.removeEventListener('visibilitychange', tick);
  };
}
