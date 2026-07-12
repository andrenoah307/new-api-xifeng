import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { getCnDisclaimer } from './api'
import {
  isStillSilent,
  markAcknowledged,
} from './lib/storage'

const ROUTE_EXEMPTIONS = ['/setup']

export function CnDisclaimerGate() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { pathname } = useLocation()
  const required = Boolean(status?.cn_disclaimer_required)
  const exempted = ROUTE_EXEMPTIONS.some((p) => pathname.startsWith(p))

  const documentQuery = useQuery({
    queryKey: ['cn-disclaimer'],
    queryFn: getCnDisclaimer,
    enabled: required && !exempted,
    staleTime: 5 * 60 * 1000,
  })

  const data = documentQuery.data
  const hash = data?.hash ?? ''
  const silenceMinutes = data?.silence_minutes ?? 0

  const [acknowledged, setAcknowledged] = useState(false)
  const [agreed, setAgreed] = useState(false)

  useEffect(() => {
    if (!hash) {
      setAcknowledged(false)
      return
    }
    setAcknowledged(isStillSilent(hash, silenceMinutes))
  }, [hash, silenceMinutes])

  if (!required || exempted) return null
  if (documentQuery.isLoading || !data) return null
  if (!data.enabled || !data.content || !hash) return null
  if (acknowledged) return null

  const handleConfirm = () => {
    markAcknowledged(hash)
    setAcknowledged(true)
  }

  return (
    <Dialog open modal>
      <DialogContent
        className='flex max-h-[80vh] max-w-2xl flex-col gap-0 p-0 [&>button.absolute]:hidden'
      >
        <DialogHeader className='border-b px-6 py-4'>
          <DialogTitle>{data.title}</DialogTitle>
        </DialogHeader>
        <div className='flex-1 overflow-y-auto px-6 py-4'>
          <div className='whitespace-pre-wrap break-words text-sm leading-6'>
            {data.content}
          </div>
        </div>
        <DialogFooter className='flex-col items-stretch gap-3 border-t px-6 py-4 sm:flex-row sm:items-center sm:justify-between'>
          <div className='flex items-start gap-2'>
            <Checkbox
              id='cn-disclaimer-agree'
              checked={agreed}
              onCheckedChange={(value) => setAgreed(value === true)}
              className='mt-0.5'
            />
            <Label
              htmlFor='cn-disclaimer-agree'
              className='text-muted-foreground text-xs leading-5 font-normal'
            >
              {t('I have read and acknowledged')}
            </Label>
          </div>
          <Button type='button' disabled={!agreed} onClick={handleConfirm}>
            {t('I acknowledge')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
