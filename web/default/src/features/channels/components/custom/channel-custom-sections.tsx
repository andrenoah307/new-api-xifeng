import { useTranslation } from 'react-i18next'
import type { UseFormReturn } from 'react-hook-form'
import { Puzzle } from 'lucide-react'
import {
  SideDrawerSectionHeader,
  sideDrawerSectionClassName,
} from '@/components/drawer-layout'
import { cn } from '@/lib/utils'
import type { ChannelFormValues } from '../../lib/channel-form'
import { PressureCoolingEditor } from './pressure-cooling-editor'
import { ChannelRateLimitEditor } from './channel-rate-limit-editor'
import { ErrorFilterRulesEditor } from './error-filter-rules-editor'
import { RiskControlHeadersEditor } from './risk-control-headers-editor'

interface CustomSectionIds {
  pressureCooling: string
  rateLimit: string
  errorFilter: string
  riskHeaders: string
}

interface CustomSectionConfigured {
  pressureCooling: boolean
  rateLimit: boolean
  errorFilter: boolean
  riskHeaders: boolean
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
  channelId?: number
  sectionIds: CustomSectionIds
  configured: CustomSectionConfigured
}

// 与 drawer 内 configuredAdvancedSectionClassName 保持同款子区块样式。
function customSubSectionClassName(configured: boolean) {
  return cn(
    'border-border/60 flex scroll-mt-4 flex-col gap-4 rounded-lg border p-3 transition-colors',
    configured && 'border-primary/35 ring-primary/20 ring-1'
  )
}

export function ChannelCustomSections({
  form,
  channelId,
  sectionIds,
  configured,
}: Props) {
  const { t } = useTranslation()

  // Enter submission is guarded by the parent channel form's bubbling handler.
  return (
    <section className={sideDrawerSectionClassName()}>
      <SideDrawerSectionHeader
        title={t('Custom Extensions')}
        description={t(
          'Pressure cooling, rate limiting, error filtering and risk-control headers for this channel.'
        )}
        icon={<Puzzle className='h-4 w-4' aria-hidden='true' />}
        iconTone='info'
      />
      <div
        id={sectionIds.pressureCooling}
        className={customSubSectionClassName(configured.pressureCooling)}
      >
        <PressureCoolingEditor form={form} />
      </div>
      <div
        id={sectionIds.rateLimit}
        className={customSubSectionClassName(configured.rateLimit)}
      >
        <ChannelRateLimitEditor form={form} />
      </div>
      <div
        id={sectionIds.errorFilter}
        className={customSubSectionClassName(configured.errorFilter)}
      >
        <ErrorFilterRulesEditor form={form} channelId={channelId} />
      </div>
      <div
        id={sectionIds.riskHeaders}
        className={customSubSectionClassName(configured.riskHeaders)}
      >
        <RiskControlHeadersEditor form={form} />
      </div>
    </section>
  )
}
