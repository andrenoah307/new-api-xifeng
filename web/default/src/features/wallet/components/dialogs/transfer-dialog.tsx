import { useState, useEffect } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='text-xl font-semibold'>
            {t('Transfer Rewards')}
          </DialogTitle>
          <DialogDescription>
            {t('Move affiliate rewards to your main balance')}
          </DialogDescription>
        </DialogHeader>

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
              {t('Minimum:')} {formatQuota(parseQuotaFromDollars(minTransferAmount))}
            </p>
          </div>
        </div>

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
