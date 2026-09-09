import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { GroupModelPerf } from '../api'
import { formatLatency, formatThroughput, formatUptimePct, getSuccessRateTextClass } from '@/features/performance-metrics/lib/format'

export default function GroupModelPerformance({ models, showAll, topN, windowHours }: { models: GroupModelPerf[]; showAll: boolean; topN: number; windowHours: number }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  if (!models.length) return <div className='text-muted-foreground text-sm'>{t('No model performance data')}</div>
  const foldable = !showAll && models.length > topN
  const visible = foldable && !expanded ? models.slice(0, topN) : models
  const hidden = models.length - topN
  return (
    <div className='space-y-2'>
      <div className='text-muted-foreground text-xs'>{t('Model performance metrics')} · {t('Last {{hours}} hours', { hours: windowHours })}</div>
      <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3'>
        {visible.map((model) => (
          <div key={model.model_name} className='bg-muted/30 rounded-lg p-2 text-xs'>
            <div className='truncate font-medium' title={model.model_name}>{model.model_name}</div>
            <div className='mt-1 grid grid-cols-2 gap-1 text-muted-foreground'>
              <span>{t('First token latency')} {formatLatency(model.avg_ttft_ms)}</span>
              <span>{t('Latency')} {formatLatency(model.avg_latency_ms)}</span>
              <span>{t('Throughput short')} {formatThroughput(model.avg_tps)}</span>
              <span className={getSuccessRateTextClass(model.success_rate)}>{t('Model success rate')} {formatUptimePct(model.success_rate)}</span>
            </div>
          </div>
        ))}
      </div>
      {foldable && (
        <button type='button' className='text-primary text-xs' onClick={() => setExpanded(!expanded)}>
          {expanded ? t('Collapse') : t('Expand all ({{count}})', { count: hidden })}
        </button>
      )}
    </div>
  )
}
