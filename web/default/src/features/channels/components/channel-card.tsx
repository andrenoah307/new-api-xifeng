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
import { GroupBadge } from '@/components/group-badge'
import { CHANNEL_STATUS } from '../constants'
import { isTagAggregateRow, parseGroupsList } from '../lib'
import type { Channel } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'

/**
 * Field columns rendered in the labelled grid (in display order). The first
 * row (`select`, `type`, `status`, `actions`), the channel name row
 * (`#id` label + `name`, aligned with `priority`/`weight`), and the
 * full-width `group` row are laid out separately around the grid.
 */
const FIELD_COLUMN_IDS = ['balance', 'response_time', 'test_time'] as const

/**
 * Bespoke channel card for the card view. Reuses every column's existing cell
 * renderer via `flexRender`, so the table's information and interactions are
 * preserved: row selection, provider/multi-key/IO.NET type badge, id,
 * name/remark + warning icons, status (with tooltips), groups, inline
 * priority/weight spinners, balance refresh, response/test times, tag
 * expand-collapse, and the per-row (or per-tag) actions menu.
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
    balance: t('Used / Remaining'),
    response_time: t('Response'),
    test_time: t('Last Tested'),
  }

  const groups = parseGroupsList(row.original.group ?? '')

  const selectCell = renderCell('select')
  const typeCell = renderCell('type')
  const nameCell = renderCell('name')
  const statusCell = renderCell('status')
  const actionsCell = renderCell('actions')
  const priorityCell = renderCell('priority')
  const weightCell = renderCell('weight')

  // In card view the enable/disable state is already conveyed by the inline
  // power toggle, so the plain "Enabled"/"Disabled" badge is redundant. Keep
  // only the informative states (e.g. auto-disabled, unknown) and tag rows.
  const showStatusBadge =
    isTagRow ||
    (row.original.status !== CHANNEL_STATUS.ENABLED &&
      row.original.status !== CHANNEL_STATUS.MANUAL_DISABLED)

  return (
    <div className='flex flex-col gap-3'>
      {/* Row 1: selection + type, with status badge + actions menu */}
      <div className='flex items-center justify-between gap-2'>
        <div className='flex min-w-0 flex-1 items-center gap-2'>
          {!isTagRow && selectCell && (
            <span className='flex-shrink-0'>{selectCell}</span>
          )}
          <div className='min-w-0 overflow-hidden'>{typeCell}</div>
        </div>
        <div className='flex flex-shrink-0 items-center gap-1.5'>
          {showStatusBadge && statusCell}
          <ChannelRowActionsLayoutContext.Provider value='card'>
            {actionsCell}
          </ChannelRowActionsLayoutContext.Provider>
        </div>
      </div>

      {/* Row 2: channel id + name (left) aligned with priority/weight (right) */}
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 flex-1 overflow-hidden text-sm'>
          {!isTagRow && (
            <div className='text-muted-foreground text-[11px] font-medium tracking-wide uppercase select-none'>
              #{row.original.id}
            </div>
          )}
          {nameCell}
        </div>
        <div className='grid flex-shrink-0 grid-cols-2 items-center gap-x-3 text-center'>
          <span className='text-muted-foreground text-[11px] font-medium tracking-wide uppercase select-none'>
            {t('Priority')}
          </span>
          <span className='text-muted-foreground text-[11px] font-medium tracking-wide uppercase select-none'>
            {t('Weight')}
          </span>
          <div className='flex justify-center'>{priorityCell}</div>
          <div className='flex justify-center'>{weightCell}</div>
        </div>
      </div>

      {/* Body: labelled fields for the remaining columns. Three fields share a
          single row at every width (the card is wide enough even on phones). */}
      <div className='grid grid-cols-3 gap-x-4 gap-y-3'>
        {FIELD_COLUMN_IDS.map((id) => {
          const content = renderCell(id)
          return (
            <div key={id} className='min-w-0'>
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

      {/* Last row: groups span the full width, showing every group (no label) */}
      <div className='min-w-0'>
        {groups.length > 0 ? (
          <div className='-ml-1.5 flex flex-wrap gap-1'>
            {groups.map((g) => (
              <GroupBadge key={g} group={g} size='sm' />
            ))}
          </div>
        ) : (
          <span className='text-muted-foreground text-sm'>-</span>
        )}
      </div>
    </div>
  )
}
