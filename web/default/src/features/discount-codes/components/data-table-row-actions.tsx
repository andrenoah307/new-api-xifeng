import type { Row } from '@tanstack/react-table'
import {
  Trash2,
  Edit,
  Power,
  PowerOff,
  Eraser,
  Loader2,
  MoreHorizontal as DotsHorizontalIcon,
} from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { handleServerError } from '@/lib/handle-server-error'

import { updateDiscountCodeStatus, cleanupDiscountCodeOrders } from '../api'
import { DISCOUNT_CODE_STATUS, SUCCESS_MESSAGES } from '../constants'
import { formatDiscountCodePendingDuration } from '../lib/pending-duration'
import { discountCodeSchema } from '../types'
import { useDiscountCodes } from './discount-codes-provider'

interface DataTableRowActionsProps<TData> {
  row: Row<TData>
  pendingTTLSeconds?: number
}

export function DataTableRowActions<TData>({
  row,
  pendingTTLSeconds,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const discountCode = discountCodeSchema.parse(row.original)
  const { setOpen, setCurrentRow, triggerRefresh } = useDiscountCodes()
  const isEnabled = discountCode.status === DISCOUNT_CODE_STATUS.ENABLED
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const [isCleaning, setIsCleaning] = useState(false)
  const cleanupInFlight = useRef(false)
  const duration = formatDiscountCodePendingDuration(pendingTTLSeconds, t)

  const handleToggleStatus = async () => {
    const newStatus = isEnabled
      ? DISCOUNT_CODE_STATUS.DISABLED
      : DISCOUNT_CODE_STATUS.ENABLED

    const result = await updateDiscountCodeStatus(discountCode.id, newStatus)
    if (result.success) {
      const message = isEnabled
        ? t(SUCCESS_MESSAGES.DISCOUNT_CODE_DISABLED)
        : t(SUCCESS_MESSAGES.DISCOUNT_CODE_ENABLED)
      toast.success(message)
      triggerRefresh()
    }
  }

  const handleCleanup = async () => {
    if (!duration || cleanupInFlight.current) return
    cleanupInFlight.current = true
    setIsCleaning(true)
    try {
      const result = await cleanupDiscountCodeOrders(discountCode.id)
      if (result.success) {
        toast.success(
          t('Marked {{count}} timed-out pending orders as expired', {
            count: result.data ?? 0,
          })
        )
        setCleanupOpen(false)
        triggerRefresh()
      } else {
        toast.error(result.message || t('Something went wrong!'))
      }
    } catch (error) {
      handleServerError(error)
    } finally {
      cleanupInFlight.current = false
      setIsCleaning(false)
    }
  }

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              className='data-popup-open:bg-muted flex h-8 w-8 p-0'
            />
          }
        >
          <DotsHorizontalIcon className='h-4 w-4' />
          <span className='sr-only'>{t('Open menu')}</span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-64'>
          <DropdownMenuItem
            onClick={() => {
              setCurrentRow(discountCode)
              setOpen('update')
            }}
          >
            {t('Edit')}
            <DropdownMenuShortcut>
              <Edit size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={handleToggleStatus}>
            {isEnabled ? (
              <>
                {t('Disable')}
                <DropdownMenuShortcut>
                  <PowerOff size={16} />
                </DropdownMenuShortcut>
              </>
            ) : (
              <>
                {t('Enable')}
                <DropdownMenuShortcut>
                  <Power size={16} />
                </DropdownMenuShortcut>
              </>
            )}
          </DropdownMenuItem>
          <DropdownMenuItem
            className='items-start whitespace-normal'
            disabled={!duration || isCleaning}
            onClick={() => setCleanupOpen(true)}
          >
            {t('Mark timed-out orders')}
            <DropdownMenuShortcut>
              <Eraser size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
          {!duration && (
            <p className='text-muted-foreground px-2 py-1 text-xs'>
              {t(
                'Pending order timeout unavailable. Refresh the list to continue.'
              )}
            </p>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => {
              setCurrentRow(discountCode)
              setOpen('delete')
            }}
            className='text-destructive focus:text-destructive'
          >
            {t('Delete')}
            <DropdownMenuShortcut>
              <Trash2 size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        open={cleanupOpen}
        onOpenChange={(open) => {
          if (!cleanupInFlight.current) setCleanupOpen(open)
        }}
        title={t('Mark timed-out orders')}
        desc={
          duration
            ? t(
                'Mark pending orders for this discount code created more than {{duration}} ago as expired? This does not cancel payment.',
                { duration }
              )
            : t(
                'Pending order timeout unavailable. Refresh the list to continue.'
              )
        }
        confirmText={
          <>
            {isCleaning && <Loader2 className='size-4 shrink-0 animate-spin' />}
            {t('Mark timed-out orders')}
          </>
        }
        isLoading={isCleaning}
        disabled={!duration}
        handleConfirm={() => {
          void handleCleanup()
        }}
      />
    </>
  )
}
