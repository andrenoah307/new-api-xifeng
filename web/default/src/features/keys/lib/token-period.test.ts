import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TFunction } from 'i18next'

import type { TokenPeriodType } from '../types'
import {
  API_KEY_FORM_DEFAULT_VALUES,
  getApiKeyFormDefaultValues,
  getApiKeyFormSchema,
  transformApiKeyToFormDefaults,
  transformFormDataToPayload,
} from './api-key-form'
import {
  canonicalQuotaToCny,
  canonicalQuotaToCnyString,
  cnyToCanonicalQuota,
  convertPeriodLimitUnit,
  formatCnyAmount,
  formatPeriodQuotaValue,
  formatPeriodResetAt,
  getPeriodBounds,
  getPeriodResetAt,
  isPositiveDecimalString,
  isPositiveIntegerString,
  roundHalfAwayFromZero,
} from './token-period'

const conversion = { usdExchangeRate: 7.3, quotaPerUnit: 500_000 }
const translate = ((key: string) => key) as unknown as TFunction

const fixture = [
  { cny: '10.00', rate: 7.3, quotaPerUnit: 500_000, expected: 684_932 },
  { cny: '0.01', rate: 7.3, quotaPerUnit: 500_000, expected: 685 },
  { cny: '1.00', rate: 7.3, quotaPerUnit: 500_000, expected: 68_493 },
  { cny: '0.000001', rate: 7.3, quotaPerUnit: 500_000, expected: 0 },
  {
    cny: '30000',
    rate: 7.3,
    quotaPerUnit: 500_000,
    expected: 2_054_794_521,
  },
] as const

describe('fixed CNY quota conversion', () => {
  test('matches the shared canonical quota fixture', () => {
    for (const entry of fixture) {
      assert.equal(
        cnyToCanonicalQuota(entry.cny, entry.rate, entry.quotaPerUnit),
        entry.expected,
        `${entry.cny}/${entry.rate}/${entry.quotaPerUnit}`
      )
    }
  })

  test('rounds half away from zero, including decimal strings', () => {
    assert.equal(roundHalfAwayFromZero(1.5), 2)
    assert.equal(roundHalfAwayFromZero(-1.5), -2)
    assert.equal(roundHalfAwayFromZero(1.49), 1)
    assert.equal(cnyToCanonicalQuota('1e-6', 1, 1_000_000), 1)
    assert.equal(cnyToCanonicalQuota('1e6', 1, 1), 1_000_000)
    assert.equal(cnyToCanonicalQuota('1', 0, 500_000), 0)
    assert.equal(cnyToCanonicalQuota('1', -1, 500_000), -500_000)
    assert.equal(cnyToCanonicalQuota('bad', 7.3, 500_000), 0)
    assert.equal(isPositiveDecimalString('NaN'), false)
    assert.equal(isPositiveDecimalString('1e1001'), false)
  })

  test('converts canonical quota back to a stable CNY string', () => {
    assert.equal(canonicalQuotaToCny(684_932, 7.3, 500_000), 10.0000072)
    assert.equal(canonicalQuotaToCnyString(684_932, 7.3, 500_000), '10.0000072')
    assert.equal(canonicalQuotaToCnyString(0, 7.3, 500_000), '0')
    assert.equal(canonicalQuotaToCnyString(10, 0, 500_000), '0')
  })

  test('keeps canonical quota stable when switching units repeatedly', () => {
    const quota = convertPeriodLimitUnit('10.00', 'cny', 'quota', conversion)
    const cny = convertPeriodLimitUnit(
      quota.value,
      'quota',
      'cny',
      conversion,
      quota.canonicalQuota
    )
    const quotaAgain = convertPeriodLimitUnit(
      cny.value,
      'cny',
      'quota',
      conversion,
      cny.canonicalQuota
    )
    assert.equal(quota.canonicalQuota, 684_932)
    assert.equal(cny.canonicalQuota, quota.canonicalQuota)
    assert.deepEqual(quotaAgain, quota)
  })

  test('validates decimal and native quota input strings', () => {
    assert.equal(isPositiveDecimalString('0.01'), true)
    assert.equal(isPositiveDecimalString('0'), false)
    assert.equal(isPositiveDecimalString('-1'), false)
    assert.equal(isPositiveDecimalString('not-a-number'), false)
    assert.equal(isPositiveIntegerString('1'), true)
    assert.equal(isPositiveIntegerString('01'), true)
    assert.equal(isPositiveIntegerString('1.0'), false)
    assert.equal(isPositiveIntegerString('0'), false)
  })
})

describe('token period form contract', () => {
  test('conditionally validates enabled periods and native quota integers', () => {
    const schema = getApiKeyFormSchema(translate)
    const base = { ...API_KEY_FORM_DEFAULT_VALUES, name: 'key' }

    assert.equal(getApiKeyFormDefaultValues(true).group, 'auto')
    assert.equal(getApiKeyFormDefaultValues(false).group, '')
    assert.equal(schema.safeParse(base).success, true)
    assert.equal(
      schema.safeParse({
        ...base,
        unlimited_quota: false,
        remain_quota_dollars: undefined,
      }).success,
      false
    )
    assert.equal(
      schema.safeParse({
        ...base,
        period_type: 'days',
        period_days: 0,
        period_limit_value: '10',
      }).success,
      false
    )
    assert.equal(
      schema.safeParse({
        ...base,
        period_type: 'week',
        period_limit_value: '0',
      }).success,
      false
    )
    assert.equal(
      schema.safeParse({
        ...base,
        period_type: 'week',
        period_limit_unit: 'quota',
        period_limit_value: '1.5',
      }).success,
      false
    )
    assert.equal(
      schema.safeParse({
        ...base,
        period_type: 'month',
        period_limit_unit: 'quota',
        period_limit_value: '1000',
      }).success,
      true
    )
  })

  test('transforms form values without sending server-owned counters', () => {
    const payload = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'periodic',
      period_type: 'days',
      period_days: 3,
      period_limit_unit: 'cny',
      period_limit_value: '10.00',
      period_reset_at: 123,
    })
    assert.deepEqual(
      {
        period_type: payload.period_type,
        period_days: payload.period_days,
        period_limit_unit: payload.period_limit_unit,
        period_limit_value: payload.period_limit_value,
      },
      {
        period_type: 'days',
        period_days: 3,
        period_limit_unit: 'cny',
        period_limit_value: '10.00',
      }
    )
    assert.equal('period_used_quota' in payload, false)
    assert.equal('period_start_at' in payload, false)
    assert.equal('period_anchor_at' in payload, false)

    const disabled = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      period_type: '',
      period_days: 99,
      period_limit_value: '12',
    })
    assert.equal(disabled.period_type, '')
    assert.equal(disabled.period_days, 0)
    assert.equal(disabled.period_limit_value, '0')
  })

  test('hydrates CNY and native quota values from canonical responses', () => {
    const apiKey = {
      id: 1,
      name: 'periodic',
      key: 'masked',
      status: 1,
      remain_quota: 0,
      used_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      created_time: 1,
      accessed_time: 1,
      group: '',
      cross_group_retry: false,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      period_type: 'week' as const,
      period_days: 0,
      period_quota_limit: 684_932,
      period_limit_unit: 'cny' as const,
      period_used_quota: 123,
      period_reset_at: 1_800_000_000,
      period_remaining_quota: 684_809,
    }
    const cnyDefaults = transformApiKeyToFormDefaults(apiKey, conversion)
    assert.equal(cnyDefaults.period_limit_value, '10.0000072')
    assert.equal(cnyDefaults.period_type, 'week')
    assert.equal(cnyDefaults.period_reset_at, 1_800_000_000)

    const quotaDefaults = transformApiKeyToFormDefaults(
      { ...apiKey, period_limit_unit: 'quota', period_quota_limit: 123_456 },
      conversion
    )
    assert.equal(quotaDefaults.period_limit_value, '123456')

    const disabledDefaults = transformApiKeyToFormDefaults(
      { ...apiKey, period_type: '', period_quota_limit: 0 },
      conversion
    )
    assert.equal(disabledDefaults.period_limit_value, '0')
    assert.equal(disabledDefaults.period_reset_at, 0)
  })
})

describe('token period boundaries and display', () => {
  const monday = Date.UTC(2026, 7, 3, 0, 0, 0) - 8 * 60 * 60 * 1000
  const sunday = monday + 6 * 24 * 60 * 60 * 1000 + 23 * 60 * 60 * 1000

  test('calculates N-day buckets from a UTC+8 anchor', () => {
    const anchor = Math.floor(
      (Date.UTC(2026, 7, 1, 0, 0, 0) - 8 * 60 * 60 * 1000) / 1000
    )
    const bounds = getPeriodBounds('days', 2, monday, anchor)
    assert.deepEqual(bounds, {
      startAt: Math.floor(monday / 1000),
      resetAt: Math.floor((monday + 2 * 24 * 60 * 60 * 1000) / 1000),
    })
    assert.equal(getPeriodResetAt('days', 0, monday, anchor), 0)
  })

  test('uses Monday week and natural month boundaries', () => {
    assert.equal(
      getPeriodResetAt('week', 0, sunday),
      Math.floor((monday + 7 * 24 * 60 * 60 * 1000) / 1000)
    )
    assert.equal(
      getPeriodResetAt('week', 0, monday),
      Math.floor((monday + 7 * 24 * 60 * 60 * 1000) / 1000)
    )
    const monthEnd = Date.UTC(2026, 9, 1, 0, 0, 0) / 1000 - 8 * 60 * 60
    assert.equal(
      getPeriodResetAt('month', 0, Date.UTC(2026, 7, 31, 16, 0, 0)),
      monthEnd
    )
    assert.equal(getPeriodResetAt('', 0, monday), 0)
    assert.equal(getPeriodResetAt('invalid' as TokenPeriodType, 0, monday), 0)
  })

  test('formats reset times and both limit units', () => {
    const resetAt = Math.floor(Date.UTC(2026, 7, 3, 16, 0, 0) / 1000)
    assert.match(formatPeriodResetAt(resetAt, 'en-US'), /UTC\+8/)
    assert.equal(formatPeriodResetAt(0, 'en-US'), '-')
    assert.equal(formatCnyAmount('10', 'en-US'), '¥10.00')
    assert.equal(
      formatPeriodQuotaValue(684_932, 'cny', conversion, 'en-US'),
      '¥10.000007'
    )
    assert.equal(
      formatPeriodQuotaValue(684_932, 'quota', conversion, 'en-US'),
      '684,932'
    )
  })
})
