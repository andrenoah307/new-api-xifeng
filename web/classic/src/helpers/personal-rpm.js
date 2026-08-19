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

export function normalizePersonalRPMItems(value) {
  if (!Array.isArray(value)) return [];
  return value
    .filter(
      (item) =>
        item &&
        typeof item === 'object' &&
        typeof item.model === 'string' &&
        typeof item.group === 'string' &&
        (item.model.length > 0 || item.group.length > 0) &&
        (item.current === null ||
          (typeof item.current === 'number' &&
            Number.isFinite(item.current) &&
            item.current >= 0)) &&
        typeof item.limit === 'number' &&
        Number.isFinite(item.limit) &&
        item.limit >= 0 &&
        (item.utilization === null ||
          (typeof item.utilization === 'number' &&
            Number.isFinite(item.utilization) &&
            item.utilization >= 0)) &&
        typeof item.available === 'boolean' &&
        typeof item.unlimited === 'boolean' &&
        typeof item.over_limit === 'boolean',
    )
    .slice()
    .sort((a, b) => {
      const aCurrent =
        a.available && typeof a.current === 'number' ? a.current : null;
      const bCurrent =
        b.available && typeof b.current === 'number' ? b.current : null;
      if (aCurrent !== null && bCurrent !== null && aCurrent !== bCurrent) {
        return bCurrent - aCurrent;
      }
      if (aCurrent !== null && bCurrent === null) return -1;
      if (aCurrent === null && bCurrent !== null) return 1;
      const aIdentity = a.model || a.group;
      const bIdentity = b.model || b.group;
      return aIdentity < bIdentity ? -1 : aIdentity > bIdentity ? 1 : 0;
    });
}

export function personalRPMDisplayState(status, items) {
  if (status === 'unavailable' || status === 'overflow') return 'unavailable';
  if (status === 'empty' || items.length === 0) return 'empty';
  return 'available';
}
