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
import { Loader2 } from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'

interface TransferDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (amount: number) => Promise<boolean>
  availableQuota: number
  transferring: boolean
  minTransferAmount?: number
}

export function TransferDialog({
  open,
  onOpenChange,
  onConfirm,
  availableQuota,
  transferring,
  minTransferAmount = 1,
}: TransferDialogProps) {
  const { t } = useTranslation()
  const [displayAmount, setDisplayAmount] = useState(minTransferAmount)

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDisplayAmount(minTransferAmount)
    }
  }, [open, minTransferAmount])

  const availableDisplayAmount = quotaUnitsToDollars(availableQuota)

  const handleConfirm = async () => {
    const quotaAmount = parseQuotaFromDollars(displayAmount)
    const success = await onConfirm(quotaAmount)
    if (success) {
      onOpenChange(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Transfer Rewards')}
      description={t('Move affiliate rewards to your main balance')}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='text-xl font-semibold'
      footerClassName='grid grid-cols-2 gap-2 sm:flex'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={transferring}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={transferring}>
            {transferring && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Transfer')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-3 sm:space-y-6 sm:py-4'>
        <div className='space-y-2'>
          <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
            {t('Transferable Rewards')}
          </Label>
          <div className='text-2xl font-semibold'>
            {formatQuota(availableQuota)}
          </div>
        </div>

        <div className='space-y-3'>
          <Label
            htmlFor='transfer-amount'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Transfer Amount')}
          </Label>
          <Input
            id='transfer-amount'
            type='number'
            value={displayAmount}
            onChange={(e) => setDisplayAmount(Number(e.target.value))}
            min={minTransferAmount}
            max={availableDisplayAmount}
            step={0.01}
            className='font-mono text-lg'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Minimum:')}{' '}
            {formatQuota(parseQuotaFromDollars(minTransferAmount))}
          </p>
        </div>
      </div>
    </Dialog>
  )
}
