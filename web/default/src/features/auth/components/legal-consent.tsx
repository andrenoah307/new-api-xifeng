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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getReadStatus,
  markRead,
  type LegalDocKey,
} from '@/features/auth/lib/legal-consent-storage'
import {
  getUserAgreement,
  LegalDocumentInline,
  type LegalDocumentResponse,
} from '@/features/legal'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'
import { LegalConsentRow } from './legal-consent-row'
import {
  createLegalConsentFeedbackState,
  reduceLegalConsentFeedbackState,
} from './legal-consent-shake'

interface LegalConsentProps {
  status: SystemStatus | null
  onAllAgreedChange?: (allAgreed: boolean) => void
  className?: string
  shakeSignal?: number
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
  shakeSignal = 0,
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
  const [feedbackState, dispatchFeedback] = useReducer(
    reduceLegalConsentFeedbackState,
    undefined,
    createLegalConsentFeedbackState
  )
  const agreedRef = useRef(agreed)
  agreedRef.current = agreed

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

  useEffect(() => {
    if (shakeSignal <= 0) return
    dispatchFeedback({
      type: 'validation-requested',
      agreed: agreedRef.current,
    })
  }, [shakeSignal])

  if (!consentRequired) return null

  const activeDoc = openDoc
    ? DOC_CONFIGS.find((d) => d.key === openDoc)
    : undefined

  const handleConfirmRead = () => {
    if (!openDoc || !hash) {
      setOpenDoc(null)
      return
    }
    const confirmedDoc = openDoc
    markRead(confirmedDoc, hash)
    setAgreed((prev) => ({ ...prev, [confirmedDoc]: true }))
    dispatchFeedback({ type: 'document-agreed', key: confirmedDoc })
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
          return (
            <LegalConsentRow
              key={doc.key}
              docKey={doc.key}
              titleKey={doc.titleKey}
              isAgreed={isAgreed}
              isDisabled={!hash}
              isInvalid={feedbackState.invalidKeys.has(doc.key)}
              isShaking={feedbackState.shakingKeys.has(doc.key)}
              onCheckedChange={(checked) => {
                if (checked) {
                  setOpenDoc(doc.key)
                } else {
                  setAgreed((prev) => ({ ...prev, [doc.key]: false }))
                }
              }}
              onOpen={() => setOpenDoc(doc.key)}
              onAnimationEnd={(event) =>
                dispatchFeedback({
                  type: 'animation-ended',
                  key: doc.key,
                  target: event.target,
                  currentTarget: event.currentTarget,
                })
              }
            />
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
              <div className='text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm'>
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
