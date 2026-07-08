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
import type { Table } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { CopyButton } from '@/components/copy-button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'

import { batchDeleteRedemptions } from '../api'
import { ERROR_MESSAGES } from '../constants'
import type { Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type DataTableBulkActionsProps<TData> = {
  table: Table<TData>
}

export function DataTableBulkActions<TData>({
  table,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const selectedRows = table.getSelectedRowModel().rows

  const contentToCopy = useMemo(() => {
    const selectedCodes = selectedRows.map((row) => {
      const redemption = row.original as Redemption
      return `${redemption.name}\t${redemption.key}`
    })
    return selectedCodes.join('\n')
  }, [selectedRows])

  const handleDelete = async () => {
    setIsDeleting(true)
    try {
      const ids = selectedRows.map((row) => (row.original as Redemption).id)
      const result = await batchDeleteRedemptions(ids)
      if (result.success) {
        const count = result.data || ids.length
        toast.success(
          t('Successfully deleted {{count}} redemption code(s)', { count })
        )
        table.resetRowSelection()
        triggerRefresh()
        setShowDeleteConfirm(false)
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName={t('redemption code')}>
        <CopyButton
          value={contentToCopy}
          variant='outline'
          size='icon'
          className='size-8'
          tooltip={t('Copy selected codes')}
          successTooltip={t('Codes copied!')}
          aria-label={t('Copy selected codes')}
        />
        <Button
          variant='destructive'
          size='icon'
          className='size-8'
          onClick={() => setShowDeleteConfirm(true)}
          title={t('Delete selected codes')}
          aria-label={t('Delete selected codes')}
        >
          <Trash2 />
        </Button>
      </BulkActionsToolbar>
      <ConfirmDialog
        destructive
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        handleConfirm={handleDelete}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete {{count}} redemption code(s)?', {
          count: selectedRows.length,
        })}
        desc={
          <>
            {t('You are about to delete {{count}} redemption code(s).', {
              count: selectedRows.length,
            })}{' '}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete')}
      />
    </>
  )
}
