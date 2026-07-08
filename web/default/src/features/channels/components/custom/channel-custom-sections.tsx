import { useTranslation } from 'react-i18next'
import type { UseFormReturn } from 'react-hook-form'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ChannelFormValues } from '../../lib/channel-form'
import { PressureCoolingEditor } from './pressure-cooling-editor'
import { ChannelRateLimitEditor } from './channel-rate-limit-editor'
import { ErrorFilterRulesEditor } from './error-filter-rules-editor'
import { RiskControlHeadersEditor } from './risk-control-headers-editor'

interface Props {
  form: UseFormReturn<ChannelFormValues>
  channelId?: number
}

export function ChannelCustomSections({ form, channelId }: Props) {
  const { t } = useTranslation()

  return (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle className="text-base">
          {t('Custom Extensions')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <PressureCoolingEditor form={form} />
        <ChannelRateLimitEditor form={form} />
        <ErrorFilterRulesEditor form={form} channelId={channelId} />
        <RiskControlHeadersEditor form={form} />
      </CardContent>
    </Card>
  )
}
