const KEY_PREFIX = 'cn-disclaimer-acknowledged'

const memoryFallback = new Map<string, string>()

function buildKey(hash: string): string {
  return `${KEY_PREFIX}:${hash}`
}

function safeGet(key: string): string | null {
  if (typeof window === 'undefined') return memoryFallback.get(key) ?? null
  try {
    return window.localStorage.getItem(key)
  } catch {
    return memoryFallback.get(key) ?? null
  }
}

function safeSet(key: string, value: string): void {
  if (typeof window === 'undefined') {
    memoryFallback.set(key, value)
    return
  }
  try {
    window.localStorage.setItem(key, value)
  } catch {
    memoryFallback.set(key, value)
  }
}

function safeRemove(key: string): void {
  if (typeof window === 'undefined') {
    memoryFallback.delete(key)
    return
  }
  try {
    window.localStorage.removeItem(key)
  } catch {
    memoryFallback.delete(key)
  }
}

function listKeys(): string[] {
  const keys: string[] = []
  if (typeof window !== 'undefined') {
    try {
      for (let i = 0; i < window.localStorage.length; i++) {
        const k = window.localStorage.key(i)
        if (k) keys.push(k)
      }
    } catch {
      // ignore
    }
  }
  for (const k of memoryFallback.keys()) {
    if (!keys.includes(k)) keys.push(k)
  }
  return keys
}

export function getAcknowledgedAt(hash: string): number | null {
  if (!hash) return null
  const raw = safeGet(buildKey(hash))
  if (!raw) return null
  const ts = Number.parseInt(raw, 10)
  return Number.isFinite(ts) ? ts : null
}

export function clearStaleEntries(currentHash: string): void {
  const prefix = `${KEY_PREFIX}:`
  const currentKey = buildKey(currentHash)
  for (const key of listKeys()) {
    if (key.startsWith(prefix) && key !== currentKey) {
      safeRemove(key)
    }
  }
}

export function markAcknowledged(hash: string): void {
  if (!hash) return
  clearStaleEntries(hash)
  safeSet(buildKey(hash), String(Math.floor(Date.now() / 1000)))
}

export function isStillSilent(hash: string, silenceMinutes: number): boolean {
  if (!hash) return false
  if (!Number.isFinite(silenceMinutes) || silenceMinutes <= 0) return false
  const ts = getAcknowledgedAt(hash)
  if (ts === null) return false
  const elapsedSeconds = Math.floor(Date.now() / 1000) - ts
  return elapsedSeconds >= 0 && elapsedSeconds < silenceMinutes * 60
}
