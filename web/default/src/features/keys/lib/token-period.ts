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

import type { TokenPeriodLimitUnit, TokenPeriodType } from '../types'

const UTC8_OFFSET_MS = 8 * 60 * 60 * 1000
const DAY_MS = 24 * 60 * 60 * 1000
const MAX_DECIMAL_EXPONENT = 1000

interface Fraction {
  numerator: bigint
  denominator: bigint
}

export interface PeriodConversionConfig {
  usdExchangeRate: number
  quotaPerUnit: number
}

function parseDecimal(value: number | string): Fraction | null {
  const text = typeof value === 'number' ? String(value) : value.trim()
  if (!text || text === 'NaN' || text === 'Infinity' || text === '-Infinity') {
    return null
  }
  if (text.length > 256) return null

  const match =
    /^([+-])?(?:(\d+)(?:\.(\d*))?|\.(\d+))(?:[eE]([+-]?\d+))?$/.exec(text)
  if (!match) return null

  const exponent = Number.parseInt(match[5] ?? '0', 10)
  if (
    !Number.isSafeInteger(exponent) ||
    Math.abs(exponent) > MAX_DECIMAL_EXPONENT
  ) {
    return null
  }

  const integerPart = match[2] ?? '0'
  const fractionPart = match[3] ?? match[4] ?? ''
  const digits = `${integerPart}${fractionPart}`.replace(/^0+(?=\d)/, '')
  let numerator = BigInt(digits || '0')
  let denominator = 1n
  const scale = fractionPart.length - exponent

  if (scale > 0) {
    denominator = 10n ** BigInt(scale)
  } else if (scale < 0) {
    numerator *= 10n ** BigInt(-scale)
  }

  if (match[1] === '-') numerator = -numerator
  return { numerator, denominator }
}

export function isPositiveDecimalString(value: string): boolean {
  const parsed = parseDecimal(value)
  return parsed !== null && parsed.numerator > 0n
}

export function isPositiveIntegerString(value: string): boolean {
  return /^\d+$/.test(value) && BigInt(value) > 0n
}

function divideFractions(left: Fraction, right: Fraction): Fraction | null {
  if (right.numerator === 0n) return null

  let numerator = left.numerator * right.denominator
  let denominator = left.denominator * right.numerator
  if (denominator < 0n) {
    numerator = -numerator
    denominator = -denominator
  }
  return { numerator, denominator }
}

function multiplyFractions(left: Fraction, right: Fraction): Fraction {
  return {
    numerator: left.numerator * right.numerator,
    denominator: left.denominator * right.denominator,
  }
}

function roundFractionAwayFromZero(value: Fraction): number {
  if (value.denominator <= 0n) return 0

  const negative = value.numerator < 0n
  const absoluteNumerator = negative ? -value.numerator : value.numerator
  let rounded = absoluteNumerator / value.denominator
  const remainder = absoluteNumerator % value.denominator
  if (remainder * 2n >= value.denominator) rounded += 1n

  const result = Number(negative ? -rounded : rounded)
  if (Number.isFinite(result)) return result
  return negative ? -Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER
}

/** Round a finite number using the backend's half-away-from-zero rule. */
export function roundHalfAwayFromZero(value: number | string): number {
  const fraction = parseDecimal(value)
  return fraction ? roundFractionAwayFromZero(fraction) : 0
}

/** Convert a fixed CNY amount to canonical quota units. */
export function cnyToCanonicalQuota(
  cny: number | string,
  usdExchangeRate: number,
  quotaPerUnit: number
): number {
  const cnyFraction = parseDecimal(cny)
  const rateFraction = parseDecimal(usdExchangeRate)
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit)
  if (!cnyFraction || !rateFraction || !quotaPerUnitFraction) return 0

  const usdFraction = divideFractions(cnyFraction, rateFraction)
  if (!usdFraction) return 0
  return roundFractionAwayFromZero(
    multiplyFractions(usdFraction, quotaPerUnitFraction)
  )
}

/** Convert canonical quota units to a fixed CNY amount. */
export function canonicalQuotaToCny(
  quota: number | string,
  usdExchangeRate: number,
  quotaPerUnit: number
): number {
  const quotaFraction = parseDecimal(quota)
  const rateFraction = parseDecimal(usdExchangeRate)
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit)
  if (!quotaFraction || !rateFraction || !quotaPerUnitFraction) return 0

  const usdFraction = divideFractions(quotaFraction, quotaPerUnitFraction)
  if (!usdFraction) return 0
  const cnyFraction = multiplyFractions(usdFraction, rateFraction)
  const result = Number(cnyFraction.numerator) / Number(cnyFraction.denominator)
  return Number.isFinite(result) ? result : 0
}

function fractionToDecimal(value: Fraction, maxFractionDigits: number): string {
  if (value.denominator <= 0n) return '0'

  const digits = Math.max(0, Math.min(18, Math.trunc(maxFractionDigits)))
  const negative = value.numerator < 0n
  const absoluteNumerator = negative ? -value.numerator : value.numerator
  const scale = 10n ** BigInt(digits)
  let scaled = (absoluteNumerator * scale) / value.denominator
  const remainder = (absoluteNumerator * scale) % value.denominator
  if (remainder * 2n >= value.denominator) scaled += 1n

  const scaledText = scaled.toString().padStart(digits + 1, '0')
  const integerText = scaledText.slice(0, -digits || undefined)
  if (digits === 0) return `${negative ? '-' : ''}${integerText}`

  const fractionText = scaledText.slice(-digits).replace(/0+$/, '')
  if (!fractionText) return `${negative ? '-' : ''}${integerText}`
  return `${negative ? '-' : ''}${integerText}.${fractionText}`
}

/** Return a stable decimal string for form inputs and unit toggles. */
export function canonicalQuotaToCnyString(
  quota: number | string,
  usdExchangeRate: number,
  quotaPerUnit: number,
  maxFractionDigits = 18
): string {
  const quotaFraction = parseDecimal(quota)
  const rateFraction = parseDecimal(usdExchangeRate)
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit)
  if (!quotaFraction || !rateFraction || !quotaPerUnitFraction) return '0'

  const usdFraction = divideFractions(quotaFraction, quotaPerUnitFraction)
  if (!usdFraction) return '0'
  return fractionToDecimal(
    multiplyFractions(usdFraction, rateFraction),
    maxFractionDigits
  )
}

export interface PeriodLimitUnitConversion {
  value: string
  canonicalQuota: number
}

/** Convert a displayed limit while keeping its canonical quota stable. */
export function convertPeriodLimitUnit(
  value: string,
  fromUnit: TokenPeriodLimitUnit,
  toUnit: TokenPeriodLimitUnit,
  conversion: PeriodConversionConfig,
  canonicalHint: number | null = null
): PeriodLimitUnitConversion {
  let canonicalQuota = canonicalHint
  if (canonicalQuota === null || !Number.isFinite(canonicalQuota)) {
    if (fromUnit === 'cny') {
      canonicalQuota = cnyToCanonicalQuota(
        value.trim(),
        conversion.usdExchangeRate,
        conversion.quotaPerUnit
      )
    } else if (isPositiveIntegerString(value.trim())) {
      canonicalQuota = Number(value.trim())
    } else {
      canonicalQuota = 0
    }
  }

  if (!Number.isFinite(canonicalQuota)) canonicalQuota = 0
  if (toUnit === 'cny') {
    return {
      value: canonicalQuotaToCnyString(
        canonicalQuota,
        conversion.usdExchangeRate,
        conversion.quotaPerUnit
      ),
      canonicalQuota,
    }
  }
  return {
    value: String(Math.max(0, Math.trunc(canonicalQuota))),
    canonicalQuota,
  }
}

export function formatCnyAmount(
  value: number | string,
  locale?: string | undefined
): string {
  const parsed = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(parsed)) return '-'
  const number = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(parsed)
  return `¥${number}`
}

export function formatPeriodQuotaValue(
  quota: number,
  unit: TokenPeriodLimitUnit,
  conversion: PeriodConversionConfig,
  locale?: string | undefined
): string {
  if (!Number.isFinite(quota)) return '-'
  if (unit === 'cny') {
    return formatCnyAmount(
      canonicalQuotaToCnyString(
        Math.max(0, Math.trunc(quota)),
        conversion.usdExchangeRate,
        conversion.quotaPerUnit
      ),
      locale
    )
  }
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(
    Math.max(0, Math.trunc(quota))
  )
}

function localMidnightAtOrBefore(nowMs: number): number {
  const shifted = new Date(nowMs + UTC8_OFFSET_MS)
  return (
    Date.UTC(
      shifted.getUTCFullYear(),
      shifted.getUTCMonth(),
      shifted.getUTCDate()
    ) - UTC8_OFFSET_MS
  )
}

export interface PeriodBounds {
  startAt: number
  resetAt: number
}

/** Calculate a token period bucket using the backend's fixed UTC+8 calendar. */
export function getPeriodBounds(
  periodType: TokenPeriodType,
  periodDays: number,
  nowMs = Date.now(),
  anchorAt = 0
): PeriodBounds | null {
  if (!Number.isFinite(nowMs)) return null
  if (periodType === '') return null

  const todayStart = localMidnightAtOrBefore(nowMs)
  if (periodType === 'days') {
    if (!Number.isInteger(periodDays) || periodDays < 1 || periodDays > 3650) {
      return null
    }
    const anchorMs = anchorAt > 0 ? anchorAt * 1000 : todayStart
    const effectiveAnchor = anchorMs > nowMs ? todayStart : anchorMs
    const elapsed = todayStart - effectiveAnchor
    const bucketIndex =
      elapsed >= 0 ? Math.floor(elapsed / (periodDays * DAY_MS)) : 0
    const startAt = effectiveAnchor + bucketIndex * periodDays * DAY_MS
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor((startAt + periodDays * DAY_MS) / 1000),
    }
  }

  const shifted = new Date(nowMs + UTC8_OFFSET_MS)
  if (periodType === 'week') {
    const daysSinceMonday = (shifted.getUTCDay() + 6) % 7
    const startAt = todayStart - daysSinceMonday * DAY_MS
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor((startAt + 7 * DAY_MS) / 1000),
    }
  }

  if (periodType === 'month') {
    const startAt =
      Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth(), 1) -
      UTC8_OFFSET_MS
    const resetAt =
      Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth() + 1, 1) -
      UTC8_OFFSET_MS
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor(resetAt / 1000),
    }
  }

  return null
}

export function getPeriodResetAt(
  periodType: TokenPeriodType,
  periodDays: number,
  nowMs = Date.now(),
  anchorAt = 0
): number {
  return getPeriodBounds(periodType, periodDays, nowMs, anchorAt)?.resetAt ?? 0
}

function normalizeIntlLocale(locale: string | undefined): string | undefined {
  if (!locale) return undefined
  if (locale === 'zhCN') return 'zh-CN'
  if (locale === 'zhTW') return 'zh-TW'
  try {
    return Intl.getCanonicalLocales(locale)[0]
  } catch {
    return 'en-US'
  }
}

/** Format a reset timestamp in the same fixed UTC+8 zone used by the backend. */
export function formatPeriodResetAt(resetAt: number, locale = 'en-US'): string {
  if (!Number.isFinite(resetAt) || resetAt <= 0) return '-'
  const date = new Date(resetAt * 1000)
  if (Number.isNaN(date.getTime())) return '-'
  return `${new Intl.DateTimeFormat(normalizeIntlLocale(locale), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
    timeZone: 'Asia/Shanghai',
  }).format(date)} (UTC+8)`
}
