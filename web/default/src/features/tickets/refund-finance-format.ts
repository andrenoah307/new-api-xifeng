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
export function refundCash(cents: number) {
  if (!Number.isSafeInteger(cents) || cents < 0) return '-'
  const value = BigInt(cents)
  return `¥${value / 100n}.${(value % 100n).toString().padStart(2, '0')}`
}

export function refundQuota(value: number, unit: string) {
  if (
    !Number.isSafeInteger(value) ||
    value < 0 ||
    !/^\d+(\.\d{1,18})?$/.test(unit)
  ) {
    return '-'
  }
  const [whole, fraction = ''] = unit.split('.')
  const denominator = BigInt(whole + fraction)
  if (denominator === 0n) return '-'
  const numerator = BigInt(value) * 10n ** BigInt(fraction.length) * 1000000n
  const rounded = (numerator + denominator / 2n) / denominator
  return `${rounded / 1000000n}.${(rounded % 1000000n).toString().padStart(6, '0')}`
}
