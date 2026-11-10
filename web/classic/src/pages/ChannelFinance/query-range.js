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
export function financeTime(value) {
  return new Date(value * 1000 + 8 * 3600000).toISOString().slice(0, 19);
}

export function financePeriod(period, now = Date.now()) {
  const date = new Date(now + 8 * 3600000);
  const year = date.getUTCFullYear();
  const month = date.getUTCMonth();
  const day = date.getUTCDate();
  let end = Math.floor(now / 3600000) * 3600;
  let start = end - 86400;
  if (period === 'day') {
    end = Date.UTC(year, month, day) / 1000 - 28800;
    start = end - 86400;
  } else if (period === 'week') {
    end =
      Date.UTC(year, month, day - ((date.getUTCDay() + 6) % 7)) / 1000 - 28800;
    start = end - 7 * 86400;
  } else if (period === 'half_month') {
    end = Date.UTC(year, month, day >= 16 ? 16 : 1) / 1000 - 28800;
    start = Date.UTC(year, day >= 16 ? month : month - 1, day >= 16 ? 1 : 16) / 1000 - 28800;
  } else if (period === 'month') {
    end = Date.UTC(year, month, 1) / 1000 - 28800;
    start = Date.UTC(year, month - 1, 1) / 1000 - 28800;
  } else if (period === 'quarter') {
    const quarter = Math.floor(month / 3) * 3;
    end = Date.UTC(year, quarter, 1) / 1000 - 28800;
    start = Date.UTC(year, quarter - 3, 1) / 1000 - 28800;
  } else if (period === 'year') {
    end = Date.UTC(year, 0, 1) / 1000 - 28800;
    start = Date.UTC(year - 1, 0, 1) / 1000 - 28800;
  }
  return { start, end, channel_id: 0 };
}
