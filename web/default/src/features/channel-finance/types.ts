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
export interface FinanceJob {
  id: string
  status:
    | 'pending'
    | 'running'
    | 'paused'
    | 'cancelled'
    | 'failed'
    | 'completed'
  start: number
  end: number
  channel_id: number
  cursor: number
  processed_windows: number
  total_windows: number
  error: string
  updated_at: number
  policy?: {
    fallback_mode: 'none' | 'billed_basis'
    rates: {
      channel_id: number
      ratio: string
      peak?: { start: string; end: string; timezone: string; ratio: string }
    }[]
  }
}

export interface FinanceRow {
  channel_id: number
  channel_name: string
  consume_count: number
  refund_count: number
  original_count: number
  fallback_count: number
  missing_count: number
  stale_count: number
  consume_amount: string
  refund_amount: string
  net_amount: string
  original_cost: string
  fallback_cost: string
  cost: string
  known_profit: string
  profit: string | null
  profit_margin: string | null
  cost_ratio: string | null
  coverage: string
}

export interface FinanceReport {
  job: FinanceJob
  channels: FinanceRow[]
  total: FinanceRow
}
