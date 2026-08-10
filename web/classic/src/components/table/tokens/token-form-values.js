/*
Copyright (C) 2025 QuantumNous

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

// This file owns the shape of the edit-token Form value store. It deliberately
// has no React, Semi UI, or i18n dependency so the hydration contract stays
// directly testable.
//
// Why the shape matters: Semi's `setValues` only walks currently mounted
// Fields unless `isOverride` is set, so conditionally rendered fields
// (period_type / period_days / period_limit_unit / period_limit_value) are
// silently dropped on hydration and fall back to their seed values. The modal
// therefore hydrates with `{ isOverride: true }`, which replaces the whole
// store — so the object handed to `setValues` must carry exactly the keys of
// `getTokenFormInitValues()`, no more and no less.
//
// `canonicalQuota` and `period_anchor_at` are server-owned metadata used only
// for round-tripping the amount input; they live in refs and must never enter
// the store.

import { periodResponseToForm } from './token-period';

export function getTokenFormInitValues() {
  return {
    name: '',
    remain_quota: 0,
    remain_amount: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    group: '',
    cross_group_retry: false,
    tokenCount: 1,
    period_enabled: false,
    period_type: '',
    period_days: 0,
    period_limit_unit: 'cny',
    period_limit_value: '0',
    period_reset_at: 0,
  };
}

// `displayed` carries the two fields whose formatting lives in src/helpers
// (which pulls in Semi UI): the expiry string and the balance in the site
// display currency. Everything else is derived here.
export function buildTokenFormValues(token = {}, periodConversion, displayed = {}) {
  const period = periodResponseToForm(token, periodConversion);
  const modelLimits = token.model_limits
    ? token.model_limits.split(',').filter(Boolean)
    : [];

  return {
    ...getTokenFormInitValues(),
    name: token.name || '',
    remain_quota: Number(token.remain_quota) || 0,
    remain_amount: displayed.remainAmount ?? 0,
    expired_time: displayed.expiredTime ?? -1,
    unlimited_quota: Boolean(token.unlimited_quota),
    model_limits_enabled: Boolean(token.model_limits_enabled),
    model_limits: modelLimits,
    allow_ips: token.allow_ips || '',
    group: token.group || '',
    cross_group_retry: Boolean(token.cross_group_retry),
    period_enabled: period.period_enabled,
    period_type: period.period_type,
    period_days: period.period_days,
    period_limit_unit: period.period_limit_unit,
    period_limit_value: period.period_limit_value,
    period_reset_at: period.period_reset_at,
  };
}

// A late response from a superseded request must not backfill the form.
export function isCurrentTokenLoadRequest(requestSequence, latestSequence) {
  return requestSequence === latestSequence;
}
