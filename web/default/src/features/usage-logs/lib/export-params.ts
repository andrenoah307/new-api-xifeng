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

import { LOG_TYPE_ALL_VALUE } from '../constants'
import type { CommonLogFilters, LogExportParams } from '../types'

export function buildExportParams(
  filters: CommonLogFilters,
  logType: string | undefined,
  isAdmin: boolean
): LogExportParams | null {
  if (!filters.startTime || !filters.endTime) return null

  const params: LogExportParams = {
    start_timestamp: Math.floor(filters.startTime.getTime() / 1000),
    end_timestamp: Math.floor(filters.endTime.getTime() / 1000),
  }

  if (logType && logType !== LOG_TYPE_ALL_VALUE) {
    params.type = Number(logType)
  }
  if (filters.model) params.model_name = filters.model
  if (filters.token) params.token_name = filters.token
  if (filters.group) params.group = filters.group
  if (filters.requestId) params.request_id = filters.requestId
  if (filters.upstreamRequestId) {
    params.upstream_request_id = filters.upstreamRequestId
  }

  if (isAdmin) {
    if (filters.username) params.username = filters.username
    if (filters.channel) params.channel = Number(filters.channel)
  }

  return params
}
