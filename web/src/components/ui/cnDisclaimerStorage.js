const KEY_PREFIX = 'cn-disclaimer-acknowledged';

const memoryFallback = new Map();

function buildKey(hash) {
  return `${KEY_PREFIX}:${hash}`;
}

function safeGet(key) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return memoryFallback.has(key) ? memoryFallback.get(key) : null;
  }
}

function safeSet(key, value) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    memoryFallback.set(key, value);
  }
}

function safeRemove(key) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    memoryFallback.delete(key);
  }
}

function listKeys() {
  const keys = [];
  try {
    for (let i = 0; i < window.localStorage.length; i++) {
      const k = window.localStorage.key(i);
      if (k) keys.push(k);
    }
  } catch {
    // ignore
  }
  for (const k of memoryFallback.keys()) {
    if (!keys.includes(k)) keys.push(k);
  }
  return keys;
}

export function getAcknowledgedAt(hash) {
  if (!hash) return null;
  const raw = safeGet(buildKey(hash));
  if (!raw) return null;
  const ts = Number.parseInt(raw, 10);
  return Number.isFinite(ts) ? ts : null;
}

export function clearStaleEntries(currentHash) {
  const prefix = `${KEY_PREFIX}:`;
  const currentKey = buildKey(currentHash);
  for (const key of listKeys()) {
    if (key.startsWith(prefix) && key !== currentKey) {
      safeRemove(key);
    }
  }
}

export function markAcknowledged(hash) {
  if (!hash) return;
  clearStaleEntries(hash);
  safeSet(buildKey(hash), String(Math.floor(Date.now() / 1000)));
}

export function isStillSilent(hash, silenceMinutes) {
  if (!hash) return false;
  if (!Number.isFinite(silenceMinutes) || silenceMinutes <= 0) return false;
  const ts = getAcknowledgedAt(hash);
  if (ts === null) return false;
  const elapsed = Math.floor(Date.now() / 1000) - ts;
  return elapsed >= 0 && elapsed < silenceMinutes * 60;
}
