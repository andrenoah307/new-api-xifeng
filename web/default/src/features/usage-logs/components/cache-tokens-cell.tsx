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

import { BookOpen, PenLine } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { CacheTokenSummary } from '../lib/cache-tokens'

interface CacheTokensInlineProps {
  summary: CacheTokenSummary
}

export function CacheTokensInline(props: CacheTokensInlineProps) {
  const { t } = useTranslation()

  if (!props.summary.hasAny) return null

  const readLabel = `${t('Cache Read')} ${props.summary.read.toLocaleString()}`
  const writeLabel = `${t('Cache Write')} ${props.summary.writeTotal.toLocaleString()}`

  return (
    <TooltipProvider delay={300}>
      <div className='flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] leading-none'>
        {props.summary.read > 0 && (
          <Tooltip>
            <TooltipTrigger
              render={
                <span
                  className='inline-flex items-center gap-1'
                  aria-label={readLabel}
                />
              }
            >
              <BookOpen
                className='text-info size-3 shrink-0'
                aria-hidden='true'
              />
              <span className='tabular-nums'>
                {props.summary.read.toLocaleString()}
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('Cache Read')}</TooltipContent>
          </Tooltip>
        )}
        {props.summary.writeTotal > 0 && (
          <Tooltip>
            <TooltipTrigger
              render={
                <span
                  className='inline-flex items-center gap-1'
                  aria-label={writeLabel}
                />
              }
            >
              <PenLine
                className='text-warning size-3 shrink-0'
                aria-hidden='true'
              />
              <span className='tabular-nums'>
                {props.summary.writeTotal.toLocaleString()}
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('Cache Write')}</TooltipContent>
          </Tooltip>
        )}
      </div>
    </TooltipProvider>
  )
}
