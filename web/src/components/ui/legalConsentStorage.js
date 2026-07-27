const KEY_PREFIX = 'legal-read';

const memoryFallback = new Map();

function buildKey(docKey, hash) {
  return `${KEY_PREFIX}:${docKey}:${hash}`;
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

export function getReadStatus(docKey, hash) {
  if (!hash) return false;
  return safeGet(buildKey(docKey, hash)) === '1';
}

export function clearStaleEntries(docKey, currentHash) {
  const prefix = `${KEY_PREFIX}:${docKey}:`;
  const currentKey = buildKey(docKey, currentHash);
  for (const key of listKeys()) {
    if (key.startsWith(prefix) && key !== currentKey) {
      safeRemove(key);
    }
  }
}

export function markRead(docKey, hash) {
  if (!hash) return;
  clearStaleEntries(docKey, hash);
  safeSet(buildKey(docKey, hash), '1');
}
