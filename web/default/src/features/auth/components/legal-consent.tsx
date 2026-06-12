import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
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
import { Skeleton } from '@/components/ui/skeleton'
import { getUserAgreement, LegalDocumentInline } from '@/features/legal'
import type { LegalDocumentResponse } from '@/features/legal'
import {
  getReadStatus,
  markRead,
  type LegalDocKey,
} from '@/features/auth/lib/legal-consent-storage'
import type { SystemStatus } from '../types'

interface LegalConsentProps {
  status: SystemStatus | null
  onAllAgreedChange?: (allAgreed: boolean) => void
  className?: string
}

type DocConfig = {
  key: LegalDocKey
  titleKey: string
}

const DOC_CONFIGS: DocConfig[] = [
  { key: 'user-agreement', titleKey: 'User Agreement' },
  { key: 'privacy-policy', titleKey: 'Privacy Policy' },
  { key: 'terms-of-service', titleKey: 'Terms of Service' },
]

export function LegalConsent({
  status,
  onAllAgreedChange,
  className,
}: LegalConsentProps) {
  const { t } = useTranslation()
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)
  const consentRequired = hasUserAgreement || hasPrivacyPolicy

  const documentQuery = useQuery<LegalDocumentResponse>({
    queryKey: ['legal-consent', 'user-agreement'],
    queryFn: getUserAgreement,
    enabled: consentRequired,
    staleTime: 5 * 60 * 1000,
  })

  const hash = documentQuery.data?.hash ?? ''
  const rawContent = documentQuery.data?.data ?? ''

  const [agreed, setAgreed] = useState<Record<LegalDocKey, boolean>>({
    'user-agreement': false,
    'privacy-policy': false,
    'terms-of-service': false,
  })
  const [openDoc, setOpenDoc] = useState<LegalDocKey | null>(null)

  useEffect(() => {
    if (!consentRequired) {
      onAllAgreedChange?.(true)
      return
    }
    if (!hash) {
      onAllAgreedChange?.(false)
      return
    }
    setAgreed({
      'user-agreement': getReadStatus('user-agreement', hash),
      'privacy-policy': getReadStatus('privacy-policy', hash),
      'terms-of-service': getReadStatus('terms-of-service', hash),
    })
  }, [hash, consentRequired, onAllAgreedChange])

  const allAgreed = useMemo(() => {
    if (!consentRequired) return true
    if (!hash) return false
    return DOC_CONFIGS.every((d) => agreed[d.key])
  }, [agreed, consentRequired, hash])

  useEffect(() => {
    onAllAgreedChange?.(allAgreed)
  }, [allAgreed, onAllAgreedChange])

  if (!consentRequired) return null

  const activeDoc = openDoc
    ? DOC_CONFIGS.find((d) => d.key === openDoc)
    : undefined

  const handleConfirmRead = () => {
    if (!openDoc || !hash) {
      setOpenDoc(null)
      return
    }
    markRead(openDoc, hash)
    setAgreed((prev) => ({ ...prev, [openDoc]: true }))
    setOpenDoc(null)
  }

  return (
    <div
      className={cn(
        'border-border/60 bg-muted/40 flex flex-col gap-2 rounded-md border p-3',
        className
      )}
    >
      {documentQuery.isLoading && (
        <div className='space-y-2'>
          <Skeleton className='h-4 w-1/2' />
          <Skeleton className='h-4 w-2/3' />
          <Skeleton className='h-4 w-1/3' />
        </div>
      )}

      {!documentQuery.isLoading &&
        DOC_CONFIGS.map((doc) => {
          const isAgreed = agreed[doc.key]
          const checkboxId = `legal-consent-${doc.key}`
          return (
            <div key={doc.key} className='flex items-start gap-2'>
              <Checkbox
                id={checkboxId}
                checked={isAgreed}
                disabled={!hash}
                onCheckedChange={(value) => {
                  if (value === true) {
                    setOpenDoc(doc.key)
                  } else {
                    setAgreed((prev) => ({ ...prev, [doc.key]: false }))
                  }
                }}
                className='mt-0.5'
              />
              <Label
                htmlFor={checkboxId}
                className='text-muted-foreground items-start gap-1 text-left text-xs leading-5 font-normal'
              >
                <span>
                  {t('I have read and agree to the')}{' '}
                  <button
                    type='button'
                    onClick={(e) => {
                      e.preventDefault()
                      setOpenDoc(doc.key)
                    }}
                    className='text-primary hover:underline'
                  >
                    {t(doc.titleKey)}
                  </button>
                </span>
              </Label>
            </div>
          )
        })}

      <Dialog
        open={openDoc !== null}
        onOpenChange={(open) => {
          if (!open) setOpenDoc(null)
        }}
      >
        <DialogContent className='flex max-h-[80vh] max-w-2xl flex-col gap-0 p-0'>
          <DialogHeader className='border-b px-6 py-4'>
            <DialogTitle>{activeDoc ? t(activeDoc.titleKey) : ''}</DialogTitle>
          </DialogHeader>
          <div className='flex-1 overflow-y-auto px-6 py-4'>
            {documentQuery.isFetching ? (
              <div className='flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground'>
                <Loader2 className='h-4 w-4 animate-spin' />
                {t('Loading...')}
              </div>
            ) : (
              <LegalDocumentInline
                rawContent={rawContent}
                emptyMessage={t('Document not configured.')}
              />
            )}
          </div>
          <DialogFooter className='border-t px-6 py-4'>
            <Button
              type='button'
              variant='outline'
              onClick={() => setOpenDoc(null)}
            >
              {t('Close')}
            </Button>
            <Button type='button' onClick={handleConfirmRead} disabled={!hash}>
              {t('I have read')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
