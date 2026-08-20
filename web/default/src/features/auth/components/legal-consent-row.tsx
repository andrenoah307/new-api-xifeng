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
import type { AnimationEventHandler } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import type { LegalDocKey } from '@/features/auth/lib/legal-consent-storage'
import { cn } from '@/lib/utils'

interface LegalConsentRowProps {
  docKey: LegalDocKey
  titleKey: string
  isAgreed: boolean
  isDisabled: boolean
  isInvalid: boolean
  isShaking: boolean
  onCheckedChange: (checked: boolean) => void
  onOpen: () => void
  onAnimationEnd: AnimationEventHandler<HTMLDivElement>
}

export function LegalConsentRow(props: LegalConsentRowProps) {
  const { t } = useTranslation()
  const checkboxId = `legal-consent-${props.docKey}`

  return (
    <div
      className={cn(
        'flex items-start gap-2',
        props.isInvalid && 'text-destructive',
        props.isShaking && 'animate-consent-shake'
      )}
      data-consent-invalid={props.isInvalid ? 'true' : undefined}
      data-consent-shake={props.isShaking ? 'true' : undefined}
      onAnimationEnd={props.onAnimationEnd}
    >
      <Checkbox
        id={checkboxId}
        checked={props.isAgreed}
        disabled={props.isDisabled}
        aria-invalid={props.isInvalid || undefined}
        onCheckedChange={(value) => props.onCheckedChange(value === true)}
        className={cn('mt-0.5', props.isInvalid && 'border-destructive')}
      />
      <Label
        htmlFor={checkboxId}
        className={cn(
          'items-start gap-1 text-left text-xs leading-5 font-normal',
          props.isInvalid ? 'text-destructive' : 'text-muted-foreground'
        )}
      >
        <span>
          {t('I have read and agree to the')}{' '}
          <button
            type='button'
            onClick={(event) => {
              event.preventDefault()
              props.onOpen()
            }}
            className={cn(
              'hover:underline',
              props.isInvalid ? 'text-destructive' : 'text-primary'
            )}
          >
            {t(props.titleKey)}
          </button>
        </span>
      </Label>
    </div>
  )
}
