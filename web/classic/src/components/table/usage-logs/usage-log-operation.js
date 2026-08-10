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

const QUOTA_OPERATION_ACTIONS = new Set([
  'user.quota_add',
  'user.quota_subtract',
  'user.quota_override',
]);

export function isUsageLogOperationAction(action) {
  return QUOTA_OPERATION_ACTIONS.has(action);
}

export function getUsageLogOperationText(
  other,
  content,
  t,
  formatQuota,
) {
  const action = other?.op?.action;
  const rawParams = other?.op?.params;
  const params =
    rawParams && typeof rawParams === 'object' && !Array.isArray(rawParams)
      ? { ...rawParams }
      : {};

  if (action === 'user.quota_add') {
    if (typeof params.quota === 'number') {
      params.quota = formatQuota(params.quota, 6);
    }
    return t('管理员增加账户额度 {{quota}}', params);
  }

  if (action === 'user.quota_subtract') {
    if (typeof params.quota === 'number') {
      params.quota = formatQuota(params.quota, 6);
    }
    return t('管理员减少账户额度 {{quota}}', params);
  }

  if (action === 'user.quota_override') {
    if (typeof params.from === 'number') {
      params.from = formatQuota(params.from, 6);
    }
    if (typeof params.to === 'number') {
      params.to = formatQuota(params.to, 6);
    }
    return t('管理员将账户额度从 {{from}} 调整为 {{to}}', params);
  }

  return content;
}
