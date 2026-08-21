/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import type { LogOtherData } from '../types'

export interface CacheTokenSummary {
  read: number
  writeGeneric: number
  write5m: number
  write1h: number
  writeTotal: number
  hasAny: boolean
}

function normalizeCacheToken(value: unknown): number {
  const token = Number(value)
  if (
    !Number.isFinite(token) ||
    token <= 0 ||
    token > Number.MAX_SAFE_INTEGER
  ) {
    return 0
  }
  return token
}

export function summarizeCacheTokens(
  other: LogOtherData | null | undefined
): CacheTokenSummary {
  const read = normalizeCacheToken(other?.cache_tokens)
  const creation = normalizeCacheToken(other?.cache_creation_tokens)
  const write5m = normalizeCacheToken(other?.cache_creation_tokens_5m)
  const write1h = normalizeCacheToken(other?.cache_creation_tokens_1h)
  const hasSplit = write5m > 0 || write1h > 0
  const writeGeneric = hasSplit
    ? Math.max(0, creation - write5m - write1h)
    : creation
  const writeTotal = writeGeneric + write5m + write1h

  return {
    read,
    writeGeneric,
    write5m,
    write1h,
    writeTotal,
    hasAny: read > 0 || writeTotal > 0,
  }
}
