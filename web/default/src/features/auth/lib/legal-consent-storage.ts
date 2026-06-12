export type LegalDocKey = 'user-agreement' | 'privacy-policy' | 'terms-of-service'

const KEY_PREFIX = 'legal-read'

const memoryFallback = new Map<string, string>()

function buildKey(docKey: LegalDocKey, hash: string): string {
  return `${KEY_PREFIX}:${docKey}:${hash}`
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

export function getReadStatus(docKey: LegalDocKey, hash: string): boolean {
  if (!hash) return false
  return safeGet(buildKey(docKey, hash)) === '1'
}

export function clearStaleEntries(docKey: LegalDocKey, currentHash: string): void {
  const prefix = `${KEY_PREFIX}:${docKey}:`
  const currentKey = buildKey(docKey, currentHash)
  for (const key of listKeys()) {
    if (key.startsWith(prefix) && key !== currentKey) {
      safeRemove(key)
    }
  }
}

export function markRead(docKey: LegalDocKey, hash: string): void {
  if (!hash) return
  clearStaleEntries(docKey, hash)
  safeSet(buildKey(docKey, hash), '1')
}
