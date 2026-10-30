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
export function formatReportAmount(value: string | number | null, key: string) {
  if (value === null || value === undefined) return '-'
  if (key.endsWith('_count')) return Number(value).toLocaleString()
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(String(value))
  if (!match) return '-'
  const percent = ['coverage', 'profit_margin', 'cost_ratio'].includes(key)
  const scale = percent ? 4 : 6
  const digits = percent ? 2 : 6
  const fraction = (match[3] ?? '').padEnd(scale + 1, '0')
  const units =
    BigInt(match[2]) * 10n ** BigInt(scale) +
    BigInt(fraction.slice(0, scale)) +
    (Number(fraction[scale]) >= 5 ? 1n : 0n)
  const divisor = 10n ** BigInt(digits)
  return `${match[1] && units !== 0n ? '-' : ''}${(units / divisor).toLocaleString()}.${(units % divisor).toString().padStart(digits, '0')}${percent ? '%' : ''}`
}
