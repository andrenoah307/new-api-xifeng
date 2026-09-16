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
import type { ChannelInflight, ChannelInflightRuntime } from '../types'

/** Age of the oldest live connection, rendered compactly enough for a table cell. */
export function formatInflightAge(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) {
    return '0s'
  }
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) {
    return `${seconds}s`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m${seconds % 60}s`
  }
  return `${Math.floor(minutes / 60)}h${minutes % 60}m`
}

/**
 * Connections that have not produced a first byte yet: the upstream accepted the
 * request but has answered with neither response headers nor a first chunk. This
 * is the number operations describes as "长连接卡住".
 */
export function countAwaitingFirstByte(inflight: ChannelInflight): number {
  return (
    Math.max(0, inflight.awaiting_headers) +
    Math.max(0, inflight.awaiting_first_chunk)
  )
}

/** Index the bulk runtime response by channel id so a row can look itself up. */
export function indexInflightByChannel(
  runtime: ChannelInflightRuntime | undefined
): Record<string, ChannelInflight> {
  const indexed: Record<string, ChannelInflight> = {}
  for (const channel of runtime?.channels ?? []) {
    indexed[String(channel.channel_id)] = channel
  }
  return indexed
}
