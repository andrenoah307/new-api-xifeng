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

const toNonNegativeCount = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
};

/** Age of the oldest live connection, rendered compactly enough for a table cell. */
export function formatInflightAge(ms) {
  const value = Number(ms);
  if (!Number.isFinite(value) || value <= 0) return '0s';
  const seconds = Math.floor(value / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h${minutes % 60}m`;
}

/**
 * Connections that have not produced a first byte yet: the upstream accepted the
 * request but has answered with neither response headers nor a first chunk. This
 * is the number operations describes as "长连接卡住".
 */
export function countAwaitingFirstByte(inflight) {
  if (!inflight || typeof inflight !== 'object') return 0;
  return (
    toNonNegativeCount(inflight.awaiting_headers) +
    toNonNegativeCount(inflight.awaiting_first_chunk)
  );
}

/**
 * Index the bulk runtime response by channel id so a row can look itself up.
 * One request per poll covers every visible row; never poll this per row.
 */
export function indexInflightByChannel(runtime) {
  const indexed = {};
  const channels = Array.isArray(runtime?.channels) ? runtime.channels : [];
  for (const channel of channels) {
    if (!channel || typeof channel !== 'object') continue;
    const id = Number(channel.channel_id);
    if (!Number.isFinite(id)) continue;
    indexed[String(id)] = channel;
  }
  return indexed;
}

/**
 * The per-class timeout contract, rendered for the confirmation dialog.
 * A class whose timeout is 0 has no overdue threshold, so its connections can
 * never become clearable — say so instead of printing a misleading "0s".
 */
export function formatInflightThreshold(seconds, enabled, notConfiguredText) {
  return enabled ? `${toNonNegativeCount(seconds)}s` : notConfiguredText;
}
