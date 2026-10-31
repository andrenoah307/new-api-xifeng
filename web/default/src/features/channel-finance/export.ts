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
import type { FinanceJob, FinanceRow } from './types'

// Keep the schema aligned with controller/channel_cost_report_csv.go.
export const reportCSVHeaders = [
  'task_id',
  'status',
  'start_timestamp',
  'end_timestamp',
  'aggregated_through',
  'channel_id',
  'channel_name',
  'consume_amount',
  'refund_amount',
  'net_amount',
  'original_cost',
  'fallback_cost',
  'cost',
  'known_profit',
  'profit',
  'profit_margin_ratio',
  'cost_ratio',
  'coverage_ratio',
  'consume_count',
  'original_count',
  'fallback_count',
  'missing_count',
  'stale_count',
  'refund_count',
]

function textCell(value: unknown) {
  const text = String(value ?? '')
  // Spreadsheet formula detection must include leading control characters.
  // eslint-disable-next-line no-control-regex
  return /^[\s\u0000-\u001f\u007f-\u009f]*[=+@-]/u.test(text) ||
    /[\t\r\n]/.test(text)
    ? "'" + text
    : text
}

export function reportCSV(job: FinanceJob, rows: FinanceRow[]) {
  const records = [
    reportCSVHeaders,
    ...rows.map((row) => [
      textCell(job.id),
      textCell(job.status),
      job.start,
      job.end,
      job.cursor,
      row.channel_id,
      textCell(row.channel_name),
      row.consume_amount,
      row.refund_amount,
      row.net_amount,
      row.original_cost,
      row.fallback_cost,
      row.cost,
      row.known_profit,
      job.status === 'completed' ? row.profit : null,
      job.status === 'completed' ? row.profit_margin : null,
      row.cost_ratio,
      row.coverage,
      row.consume_count,
      row.original_count,
      row.fallback_count,
      row.missing_count,
      row.stale_count,
      row.refund_count,
    ]),
  ]
  return (
    '\ufeff' +
    records
      .map((record) =>
        record
          .map((value) => '"' + String(value ?? '').replaceAll('"', '""') + '"')
          .join(',')
      )
      .join('\r\n') +
    '\r\n'
  )
}
