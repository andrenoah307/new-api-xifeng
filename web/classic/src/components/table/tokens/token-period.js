/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// This file deliberately has no React, i18n, or browser-storage dependency.
// Keep the arithmetic in lockstep with web/default/src/features/keys/lib/token-period.ts.
//
// The conversion config is `{ displayRate, quotaPerUnit, symbol }`, where
// `displayRate` is `1 USD = X <站点展示币种>` as configured by the admin.
// TOKENS display is encoded as `displayRate === quotaPerUnit` with an empty
// symbol, which collapses the amount conversion to identity without adding a
// branch to the arithmetic below. Callers build it with
// `getPeriodConversionConfig()` from src/helpers/quota.js.

export const TOKEN_PERIOD_MAX_DAYS = 3650;
export const TOKEN_PERIOD_TYPES = ['', 'days', 'week', 'month'];
export const TOKEN_PERIOD_LIMIT_UNITS = ['cny', 'quota'];

const UTC8_OFFSET_MS = 8 * 60 * 60 * 1000;
const DAY_MS = 24 * 60 * 60 * 1000;
const MAX_DECIMAL_EXPONENT = 1000;
const MAX_PERIOD_QUOTA = 2147483647;

function parseDecimal(value) {
  const text = (typeof value === 'number' ? String(value) : String(value ?? '')).trim();
  if (!text || text === 'NaN' || text === 'Infinity' || text === '-Infinity') {
    return null;
  }
  if (text.length > 256) return null;

  const match = /^([+-])?(?:(\d+)(?:\.(\d*))?|\.(\d+))(?:[eE]([+-]?\d+))?$/.exec(text);
  if (!match) return null;

  const exponent = Number.parseInt(match[5] ?? '0', 10);
  if (!Number.isSafeInteger(exponent) || Math.abs(exponent) > MAX_DECIMAL_EXPONENT) {
    return null;
  }

  const integerPart = match[2] ?? '0';
  const fractionPart = match[3] ?? match[4] ?? '';
  const digits = `${integerPart}${fractionPart}`.replace(/^0+(?=\d)/, '');
  let numerator = BigInt(digits || '0');
  let denominator = 1n;
  const scale = fractionPart.length - exponent;

  if (scale > 0) {
    denominator = 10n ** BigInt(scale);
  } else if (scale < 0) {
    numerator *= 10n ** BigInt(-scale);
  }

  if (match[1] === '-') numerator = -numerator;
  return { numerator, denominator };
}

function divideFractions(left, right) {
  if (right.numerator === 0n) return null;

  let numerator = left.numerator * right.denominator;
  let denominator = left.denominator * right.numerator;
  if (denominator < 0n) {
    numerator = -numerator;
    denominator = -denominator;
  }
  return { numerator, denominator };
}

function multiplyFractions(left, right) {
  return {
    numerator: left.numerator * right.numerator,
    denominator: left.denominator * right.denominator,
  };
}

function roundFractionAwayFromZero(value) {
  if (value.denominator <= 0n) return 0;

  const negative = value.numerator < 0n;
  const absoluteNumerator = negative ? -value.numerator : value.numerator;
  let rounded = absoluteNumerator / value.denominator;
  const remainder = absoluteNumerator % value.denominator;
  if (remainder * 2n >= value.denominator) rounded += 1n;

  const result = Number(negative ? -rounded : rounded);
  if (Number.isFinite(result)) return result;
  return negative ? -Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER;
}

export function roundHalfAwayFromZero(value) {
  const fraction = parseDecimal(value);
  return fraction ? roundFractionAwayFromZero(fraction) : 0;
}

export function isPositiveDecimalString(value) {
  const parsed = parseDecimal(value);
  return parsed !== null && parsed.numerator > 0n;
}

export function isPositiveIntegerString(value) {
  return /^\d+$/.test(String(value ?? '')) && BigInt(value) > 0n;
}

export function amountToCanonicalQuota(amount, displayRate, quotaPerUnit) {
  const amountFraction = parseDecimal(amount);
  const rateFraction = parseDecimal(displayRate);
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit);
  if (!amountFraction || !rateFraction || !quotaPerUnitFraction) return 0;

  const usdFraction = divideFractions(amountFraction, rateFraction);
  if (!usdFraction) return 0;
  return roundFractionAwayFromZero(
    multiplyFractions(usdFraction, quotaPerUnitFraction),
  );
}

export function canonicalQuotaToAmount(quota, displayRate, quotaPerUnit) {
  const quotaFraction = parseDecimal(quota);
  const rateFraction = parseDecimal(displayRate);
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit);
  if (!quotaFraction || !rateFraction || !quotaPerUnitFraction) return 0;

  const usdFraction = divideFractions(quotaFraction, quotaPerUnitFraction);
  if (!usdFraction) return 0;
  const amountFraction = multiplyFractions(usdFraction, rateFraction);
  const result =
    Number(amountFraction.numerator) / Number(amountFraction.denominator);
  return Number.isFinite(result) ? result : 0;
}

function fractionToDecimal(value, maxFractionDigits) {
  if (value.denominator <= 0n) return '0';

  const digits = Math.max(0, Math.min(18, Math.trunc(maxFractionDigits)));
  const negative = value.numerator < 0n;
  const absoluteNumerator = negative ? -value.numerator : value.numerator;
  const scale = 10n ** BigInt(digits);
  let scaled = (absoluteNumerator * scale) / value.denominator;
  const remainder = (absoluteNumerator * scale) % value.denominator;
  if (remainder * 2n >= value.denominator) scaled += 1n;

  const scaledText = scaled.toString().padStart(digits + 1, '0');
  const integerText = scaledText.slice(0, -digits || undefined);
  if (digits === 0) return `${negative ? '-' : ''}${integerText}`;

  const fractionText = scaledText.slice(-digits).replace(/0+$/, '');
  if (!fractionText) return `${negative ? '-' : ''}${integerText}`;
  return `${negative ? '-' : ''}${integerText}.${fractionText}`;
}

export function canonicalQuotaToAmountString(
  quota,
  displayRate,
  quotaPerUnit,
  maxFractionDigits = 18,
) {
  const quotaFraction = parseDecimal(quota);
  const rateFraction = parseDecimal(displayRate);
  const quotaPerUnitFraction = parseDecimal(quotaPerUnit);
  if (!quotaFraction || !rateFraction || !quotaPerUnitFraction) return '0';

  const usdFraction = divideFractions(quotaFraction, quotaPerUnitFraction);
  if (!usdFraction) return '0';
  return fractionToDecimal(
    multiplyFractions(usdFraction, rateFraction),
    maxFractionDigits,
  );
}

/**
 * Shortest decimal string that maps back to the same canonical quota.
 *
 * Amounts quantize onto integer quota with step `displayRate /
 * quotaPerUnit`, so the exact quotient of a stored quota is rarely the string
 * the user typed. Walk decimal precisions upward and return the first
 * candidate that the real quantizer maps back to `quota`.
 *
 * The 18-digit fallback is only reachable when no representative exists within
 * 18 digits; it returns the same value `canonicalQuotaToAmountString` does today.
 */
export function canonicalQuotaToAmountInputString(
  quota,
  displayRate,
  quotaPerUnit,
) {
  const quotaFraction = parseDecimal(quota);
  if (!quotaFraction) return '0';
  const target = roundFractionAwayFromZero(quotaFraction);
  for (let digits = 0; digits <= 18; digits++) {
    const candidate = canonicalQuotaToAmountString(
      quota,
      displayRate,
      quotaPerUnit,
      digits,
    );
    if (
      amountToCanonicalQuota(candidate, displayRate, quotaPerUnit) === target
    ) {
      return candidate;
    }
  }
  return canonicalQuotaToAmountString(quota, displayRate, quotaPerUnit);
}

export function normalizePeriodConversion(conversion = {}) {
  const displayRate = Number(conversion.displayRate);
  const quotaPerUnit = Number(conversion.quotaPerUnit);
  return {
    displayRate:
      Number.isFinite(displayRate) && displayRate > 0 ? displayRate : 1,
    quotaPerUnit:
      Number.isFinite(quotaPerUnit) && quotaPerUnit > 0 ? quotaPerUnit : 1,
    symbol: typeof conversion.symbol === 'string' ? conversion.symbol : '',
  };
}

export function convertPeriodLimitUnit(
  value,
  fromUnit,
  toUnit,
  conversion,
  canonicalHint = null,
) {
  const normalizedConversion = normalizePeriodConversion(conversion);
  let canonicalQuota = canonicalHint;
  if (canonicalQuota === null || !Number.isFinite(canonicalQuota)) {
    if (fromUnit === 'cny') {
      canonicalQuota = amountToCanonicalQuota(
        String(value ?? '').trim(),
        normalizedConversion.displayRate,
        normalizedConversion.quotaPerUnit,
      );
    } else if (isPositiveIntegerString(String(value ?? '').trim())) {
      canonicalQuota = Number(String(value).trim());
    } else {
      canonicalQuota = 0;
    }
  }

  if (!Number.isFinite(canonicalQuota)) canonicalQuota = 0;
  if (toUnit === 'cny') {
    return {
      value: canonicalQuotaToAmountInputString(
        canonicalQuota,
        normalizedConversion.displayRate,
        normalizedConversion.quotaPerUnit,
      ),
      canonicalQuota,
    };
  }
  return {
    value: String(Math.max(0, Math.trunc(canonicalQuota))),
    canonicalQuota,
  };
}

/** Format an amount with the site's display symbol; TOKENS mode has none. */
export function formatDisplayAmount(value, symbol, locale) {
  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(parsed)) return '-';
  if (!symbol) {
    return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(
      parsed,
    );
  }
  const number = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(parsed);
  return `${symbol}${number}`;
}

export function formatPeriodQuotaValue(quota, unit, conversion, locale) {
  if (!Number.isFinite(Number(quota))) return '-';
  const normalizedConversion = normalizePeriodConversion(conversion);
  const normalizedQuota = Math.max(0, Math.trunc(Number(quota)));
  if (unit === 'cny') {
    return formatDisplayAmount(
      canonicalQuotaToAmountString(
        normalizedQuota,
        normalizedConversion.displayRate,
        normalizedConversion.quotaPerUnit,
      ),
      normalizedConversion.symbol,
      locale,
    );
  }
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(
    normalizedQuota,
  );
}

/**
 * Format a period *limit* — a value the user typed — so it reads back as typed.
 * Accumulated usage keeps formatPeriodQuotaValue's exact center value.
 */
export function formatPeriodLimitValue(quota, unit, conversion, locale) {
  if (!Number.isFinite(Number(quota))) return '-';
  if (unit !== 'cny') return formatPeriodQuotaValue(quota, unit, conversion, locale);
  const normalizedConversion = normalizePeriodConversion(conversion);
  return formatDisplayAmount(
    canonicalQuotaToAmountInputString(
      Math.max(0, Math.trunc(quota)),
      normalizedConversion.displayRate,
      normalizedConversion.quotaPerUnit,
    ),
    normalizedConversion.symbol,
    locale,
  );
}

function localMidnightAtOrBefore(nowMs) {
  const shifted = new Date(nowMs + UTC8_OFFSET_MS);
  return (
    Date.UTC(
      shifted.getUTCFullYear(),
      shifted.getUTCMonth(),
      shifted.getUTCDate(),
    ) - UTC8_OFFSET_MS
  );
}

export function getPeriodBounds(
  periodType,
  periodDays,
  nowMs = Date.now(),
  anchorAt = 0,
) {
  if (!Number.isFinite(nowMs) || periodType === '') return null;

  const todayStart = localMidnightAtOrBefore(nowMs);
  if (periodType === 'days') {
    if (
      !Number.isInteger(periodDays) ||
      periodDays < 1 ||
      periodDays > TOKEN_PERIOD_MAX_DAYS
    ) {
      return null;
    }
    const anchorMs = anchorAt > 0 ? anchorAt * 1000 : todayStart;
    const effectiveAnchor = anchorMs > nowMs ? todayStart : anchorMs;
    const elapsed = todayStart - effectiveAnchor;
    const bucketIndex =
      elapsed >= 0 ? Math.floor(elapsed / (periodDays * DAY_MS)) : 0;
    const startAt = effectiveAnchor + bucketIndex * periodDays * DAY_MS;
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor((startAt + periodDays * DAY_MS) / 1000),
    };
  }

  const shifted = new Date(nowMs + UTC8_OFFSET_MS);
  if (periodType === 'week') {
    const daysSinceMonday = (shifted.getUTCDay() + 6) % 7;
    const startAt = todayStart - daysSinceMonday * DAY_MS;
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor((startAt + 7 * DAY_MS) / 1000),
    };
  }

  if (periodType === 'month') {
    const startAt =
      Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth(), 1) -
      UTC8_OFFSET_MS;
    const resetAt =
      Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth() + 1, 1) -
      UTC8_OFFSET_MS;
    return {
      startAt: Math.floor(startAt / 1000),
      resetAt: Math.floor(resetAt / 1000),
    };
  }

  return null;
}

export function getPeriodResetAt(
  periodType,
  periodDays,
  nowMs = Date.now(),
  anchorAt = 0,
) {
  return getPeriodBounds(periodType, periodDays, nowMs, anchorAt)?.resetAt ?? 0;
}

function normalizeIntlLocale(locale) {
  if (!locale) return undefined;
  if (locale === 'zhCN') return 'zh-CN';
  if (locale === 'zhTW') return 'zh-TW';
  try {
    return Intl.getCanonicalLocales(locale)[0];
  } catch {
    return 'en-US';
  }
}

export function formatPeriodResetAt(resetAt, locale = 'en-US') {
  if (!Number.isFinite(Number(resetAt)) || Number(resetAt) <= 0) return '-';
  const date = new Date(Number(resetAt) * 1000);
  if (Number.isNaN(date.getTime())) return '-';
  return `${new Intl.DateTimeFormat(normalizeIntlLocale(locale), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
    timeZone: 'Asia/Shanghai',
  }).format(date)} (UTC+8)`;
}

function isPeriodType(value) {
  return value === 'days' || value === 'week' || value === 'month';
}

function isPeriodUnit(value) {
  return value === 'cny' || value === 'quota';
}

export function periodFormToPayload(values = {}) {
  const periodType = isPeriodType(values.period_type) ? values.period_type : '';
  const periodUnit = isPeriodUnit(values.period_limit_unit)
    ? values.period_limit_unit
    : 'cny';
  const enabled = values.period_enabled === undefined
    ? periodType !== ''
    : Boolean(values.period_enabled);
  const value = String(values.period_limit_value ?? '').trim();
  // 限额填 0 / 留空即视为关闭周期限额，与后端的「禁用」分支对齐
  if (!enabled || periodType === '' || !isPositiveDecimalString(value)) {
    return {
      period_type: '',
      period_days: 0,
      period_limit_unit: periodUnit,
      period_limit_value: '0',
    };
  }

  const rawDays = Number(values.period_days);
  const periodDays =
    periodType === 'days' && Number.isInteger(rawDays) ? rawDays : 0;
  return {
    period_type: periodType,
    period_days: periodDays,
    period_limit_unit: periodUnit,
    period_limit_value: value,
  };
}

export function periodResponseToForm(token = {}, conversion) {
  const periodType = isPeriodType(token.period_type) ? token.period_type : '';
  const limit = Number(token.period_quota_limit) || 0;
  const enabled = periodType !== '' && limit > 0;
  const periodUnit = token.period_limit_unit === 'quota' ? 'quota' : 'cny';
  const normalizedConversion = normalizePeriodConversion(conversion);
  let periodLimitValue = '0';
  if (enabled && periodUnit === 'cny') {
    periodLimitValue = canonicalQuotaToAmountInputString(
      limit,
      normalizedConversion.displayRate,
      normalizedConversion.quotaPerUnit,
    );
  } else if (enabled) {
    periodLimitValue = String(Math.max(0, Math.trunc(limit)));
  }
  const rawDays = Number(token.period_days);
  return {
    period_enabled: enabled,
    period_type: enabled ? periodType : '',
    period_days: enabled && periodType === 'days' && Number.isInteger(rawDays)
      ? rawDays
      : 0,
    period_limit_unit: enabled ? periodUnit : 'cny',
    period_limit_value: periodLimitValue,
    period_reset_at: enabled ? Number(token.period_reset_at) || 0 : 0,
    canonicalQuota: enabled ? limit : null,
    period_anchor_at: enabled ? Number(token.period_anchor_at) || 0 : 0,
  };
}

export function validatePeriodForm(values = {}, conversion) {
  const payload = periodFormToPayload(values);
  // 关闭态（含限额填 0 / 留空）没有可校验的限额
  if (payload.period_type === '') return { valid: true, errors: [] };

  const normalizedConversion = normalizePeriodConversion(conversion);
  const errors = [];
  if (
    payload.period_type === 'days' &&
    (!Number.isInteger(payload.period_days) ||
      payload.period_days < 1 ||
      payload.period_days > TOKEN_PERIOD_MAX_DAYS)
  ) {
    errors.push('period_days');
  }
  const value = payload.period_limit_value;
  if (
    payload.period_limit_unit === 'quota' &&
    !isPositiveIntegerString(value)
  ) {
    errors.push('period_limit_value_integer');
  } else {
    const quota = payload.period_limit_unit === 'cny'
      ? amountToCanonicalQuota(
        value,
        normalizedConversion.displayRate,
        normalizedConversion.quotaPerUnit,
      )
      : Number(value);
    if (!Number.isSafeInteger(quota) || quota < 1 || quota > MAX_PERIOD_QUOTA) {
      errors.push('period_limit_value_range');
    }
  }
  return { valid: errors.length === 0, errors };
}
