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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

import { DEFAULT_GROUP } from '../constants'
import {
  TOKEN_PERIOD_LIMIT_UNITS,
  TOKEN_PERIOD_MAX_DAYS,
  TOKEN_PERIOD_TYPES,
  type ApiKey,
  type ApiKeyFormData,
  type TokenPeriodLimitUnit,
  type TokenPeriodType,
} from '../types'
import {
  canonicalQuotaToCnyString,
  isPositiveDecimalString,
  isPositiveIntegerString,
} from './token-period'

// ============================================================================
// Form Schema
// ============================================================================

export function getApiKeyFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(1, t('Please enter a name')),
      remain_quota_dollars: z.number().optional(),
      expired_time: z.date().optional(),
      unlimited_quota: z.boolean(),
      model_limits: z.array(z.string()),
      allow_ips: z.string().optional(),
      group: z.string().optional(),
      cross_group_retry: z.boolean().optional(),
      tokenCount: z.number().min(1).optional(),
      period_type: z.enum(TOKEN_PERIOD_TYPES),
      period_days: z
        .number()
        .int()
        .min(0)
        .max(TOKEN_PERIOD_MAX_DAYS),
      period_limit_unit: z.enum(TOKEN_PERIOD_LIMIT_UNITS),
      period_limit_value: z.string(),
      period_reset_at: z.number().optional(),
    })
    .superRefine((data, ctx) => {
      if (
        !data.unlimited_quota &&
        (data.remain_quota_dollars === undefined ||
          data.remain_quota_dollars < 0)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['remain_quota_dollars'],
          message: t('Quota must be zero or greater'),
        })
      }

      if (data.period_type === '') return

      if (
        data.period_type === 'days' &&
        (!Number.isInteger(data.period_days) ||
          data.period_days < 1 ||
          data.period_days > TOKEN_PERIOD_MAX_DAYS)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['period_days'],
          message: t('Period days must be between 1 and 3650'),
        })
      }

      const periodValue = data.period_limit_value.trim()
      if (!isPositiveDecimalString(periodValue)) {
        ctx.addIssue({
          code: 'custom',
          path: ['period_limit_value'],
          message: t('Period limit must be greater than zero'),
        })
      } else if (
        data.period_limit_unit === 'quota' &&
        !isPositiveIntegerString(periodValue)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['period_limit_value'],
          message: t('Native quota limits must be positive integers'),
        })
      }
    })
}

export type ApiKeyFormValues = z.infer<ReturnType<typeof getApiKeyFormSchema>>

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  remain_quota_dollars: 10,
  expired_time: undefined,
  unlimited_quota: true,
  model_limits: [],
  allow_ips: '',
  group: DEFAULT_GROUP,
  cross_group_retry: true,
  tokenCount: 1,
  period_type: '',
  period_days: 0,
  period_limit_unit: 'cny',
  period_limit_value: '0',
  period_reset_at: 0,
}

export function getApiKeyFormDefaultValues(
  defaultUseAutoGroup: boolean
): ApiKeyFormValues {
  return {
    ...API_KEY_FORM_DEFAULT_VALUES,
    group: defaultUseAutoGroup ? 'auto' : DEFAULT_GROUP,
    cross_group_retry: defaultUseAutoGroup,
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  const periodType = data.period_type || ''
  const periodUnit = data.period_limit_unit || 'cny'
  const periodValue = data.period_limit_value || '0'
  const periodEnabled = periodType !== ''
  return {
    name: data.name,
    // The legacy token balance still follows the site's display mode. Period
    // limits are converted separately by token-period.ts using fixed CNY.
    remain_quota: data.unlimited_quota
      ? 0
      : parseQuotaFromDollars(data.remain_quota_dollars || 0),
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    unlimited_quota: data.unlimited_quota,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    allow_ips: data.allow_ips || '',
    group: data.group || '',
    cross_group_retry: data.group === 'auto' ? !!data.cross_group_retry : false,
    period_type: periodType,
    period_days: periodEnabled && periodType === 'days' ? data.period_days : 0,
    period_limit_unit: periodUnit,
    period_limit_value: periodEnabled ? periodValue.trim() || '0' : '0',
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey,
  conversion: {
    usdExchangeRate: number
    quotaPerUnit: number
  }
): ApiKeyFormValues {
  const periodType: TokenPeriodType = TOKEN_PERIOD_TYPES.includes(
    (apiKey.period_type || '') as TokenPeriodType
  )
    ? ((apiKey.period_type || '') as TokenPeriodType)
    : ''
  const periodEnabled = periodType !== '' && apiKey.period_quota_limit > 0
  const periodUnit: TokenPeriodLimitUnit =
    apiKey.period_limit_unit === 'quota' ? 'quota' : 'cny'
  let periodLimitValue = '0'
  if (periodEnabled && periodUnit === 'cny') {
    periodLimitValue = canonicalQuotaToCnyString(
      apiKey.period_quota_limit,
      conversion.usdExchangeRate,
      conversion.quotaPerUnit
    )
  } else if (periodEnabled) {
    periodLimitValue = String(
      Math.max(0, Math.trunc(apiKey.period_quota_limit))
    )
  }

  return {
    name: apiKey.name,
    remain_quota_dollars: apiKey.unlimited_quota
      ? 0
      : quotaUnitsToDollars(apiKey.remain_quota),
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    allow_ips: apiKey.allow_ips || '',
    group: apiKey.group || DEFAULT_GROUP,
    cross_group_retry: !!apiKey.cross_group_retry,
    tokenCount: 1,
    period_type: periodEnabled ? periodType : '',
    period_days:
      periodEnabled && periodType === 'days' ? apiKey.period_days : 0,
    period_limit_unit: periodEnabled ? periodUnit : 'cny',
    period_limit_value: periodLimitValue,
    period_reset_at: periodEnabled ? apiKey.period_reset_at || 0 : 0,
  }
}
