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
/** Consumption and cost snapshots share the base pricing unit, before display conversion. */
export function calculateLogProfit(role, type, quota, quotaPerUnit, cost) {
  if (role !== 100 || type !== 2) return null;
  if (!cost || !['estimated', 'stale'].includes(cost.status)) return null;
  if (
    typeof cost.amount_usd !== 'string' ||
    !/^\d+(\.\d+)?$/.test(cost.amount_usd)
  )
    return null;
  const amount = Number(cost.amount_usd);
  if (
    !Number.isFinite(amount) ||
    !Number.isFinite(quota) ||
    !Number.isFinite(quotaPerUnit) ||
    quotaPerUnit <= 0
  )
    return null;
  const profit = quota / quotaPerUnit - amount;
  return Number.isFinite(profit) ? profit : null;
}
