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
import { flexRender, type Row } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { isTagAggregateRow } from '../lib'
import type { Channel } from '../types'

/**
 * Field columns rendered in the card body (in display order). The header
 * columns (`select`, `name`, `status`, `actions`) are laid out separately.
 * `models` spans the full width because it can hold many badges.
 */
const FIELD_COLUMN_IDS = [
  'type',
  'id',
  'group',
  'balance',
  'priority',
  'weight',
  'response_time',
  'test_time',
  'tag',
  'models',
] as const

/**
 * Bespoke channel card for the card view. Reuses every column's existing cell
 * renderer via `flexRender`, so all table information and interactions are
 * preserved: row selection, name/remark + warning icons, status (with tooltips),
 * provider/multi-key/IO.NET badges, groups, models, tag, inline priority/weight
 * spinners, balance refresh, response/test times, tag expand-collapse, and the
 * per-row (or per-tag) actions menu.
 */
export function ChannelCard({ row }: { row: Row<Channel> }) {
  const { t } = useTranslation()
  const isTagRow = isTagAggregateRow(row.original)
  const cells = row.getAllCells()

  const renderCell = (id: string) => {
    const cell = cells.find((c) => c.column.id === id)
    if (!cell || !cell.column.columnDef.cell) {
      return null
    }
    return flexRender(cell.column.columnDef.cell, cell.getContext())
  }

  const fieldLabels: Record<string, string> = {
    type: t('Type'),
    id: t('ID'),
    group: t('Groups'),
    balance: t('Used / Remaining'),
    priority: t('Priority'),
    weight: t('Weight'),
    response_time: t('Response'),
    test_time: t('Last Tested'),
    tag: t('Tag'),
    models: t('Models'),
  }

  const selectCell = renderCell('select')
  const nameCell = renderCell('name')
  const statusCell = renderCell('status')
  const actionsCell = renderCell('actions')

  return (
    <div className='flex flex-col gap-3'>
      {/* Header: selection + name/remark, with status badge + actions menu */}
      <div className='flex items-start justify-between gap-2'>
        <div className='flex min-w-0 flex-1 items-start gap-2'>
          {!isTagRow && selectCell && (
            <div className='pt-0.5'>{selectCell}</div>
          )}
          <div className='min-w-0 flex-1'>{nameCell}</div>
        </div>
        <div className='flex flex-shrink-0 items-center gap-1.5'>
          {statusCell}
          {actionsCell}
        </div>
      </div>

      {/* Body: labelled fields for every remaining column */}
      <div className='grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3'>
        {FIELD_COLUMN_IDS.map((id) => {
          const content = renderCell(id)
          return (
            <div
              key={id}
              className={cn(
                'min-w-0',
                id === 'models' && 'col-span-2 sm:col-span-3'
              )}
            >
              <div className='text-muted-foreground mb-1 text-[11px] font-medium tracking-wide uppercase select-none'>
                {fieldLabels[id]}
              </div>
              <div className='min-w-0 overflow-hidden text-sm'>
                {content ?? <span className='text-muted-foreground'>-</span>}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
