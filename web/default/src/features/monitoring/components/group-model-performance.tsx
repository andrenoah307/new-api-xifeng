import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { GroupModelPerf } from '../api'
import { formatLatency, formatThroughput, formatUptimePct, getSuccessRateTextClass } from '@/features/performance-metrics/lib/format'
import MiniSparkline from './mini-sparkline'

export interface GroupModelPerformanceProps {
  models: GroupModelPerf[]
  showAll: boolean
  topN: number
  windowHours: number
}

export default function GroupModelPerformance({
  models,
  showAll,
  topN,
  windowHours,
}: GroupModelPerformanceProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  if (!models.length) return <div className='text-muted-foreground text-sm'>{t('No model performance data')}</div>
  const foldable = !showAll && models.length > topN
  const visible = foldable && !expanded ? models.slice(0, topN) : models
  const hidden = models.length - topN
  return (
    <div className='@container/perfcols space-y-2'>
      <div className='text-muted-foreground text-xs'>{t('Model performance metrics')} · {t('Last {{hours}} hours', { hours: windowHours })}</div>
      {/* 指标名只在表头出现一次：模型数达到 20 时，逐卡重复标签既撑不下也读不动 */}
      <table className='w-full border-collapse text-xs'>
        <thead>
          <tr className='text-muted-foreground border-border/60 border-b [&>th]:px-1.5 [&>th]:py-1 [&>th]:font-normal [&>th]:whitespace-nowrap'>
            <th scope='col' className='text-left'>{t('Model')}</th>
            <th scope='col' className='text-right' title={t('First token latency')}>{t('First token latency short')}</th>
            <th scope='col' className='text-right'>{t('Latency')}</th>
            <th scope='col' className='text-right' title={t('Throughput')}>{t('Throughput short')}</th>
            <th scope='col' className='text-right' title={t('Model success rate')}>{t('Model success rate short')}</th>
            <th scope='col' className='hidden w-[76px] text-right @sm/perfcols:table-cell'>{t('Trend')}</th>
          </tr>
        </thead>
        <tbody>
          {visible.map((model) => (
            <tr key={model.model_name} className='border-border/40 hover:bg-muted/40 border-b last:border-0'>
              {/* w-full + max-w-0 让名称列吃掉剩余宽度并可截断，全名留在 title 里 */}
              <td className='w-full max-w-0 truncate px-1.5 py-1 font-medium' title={model.model_name}>{model.model_name}</td>
              <td className='px-1.5 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatLatency(model.avg_ttft_ms)}</td>
              <td className='px-1.5 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatLatency(model.avg_latency_ms)}</td>
              <td className='px-1.5 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatThroughput(model.avg_tps)}</td>
              <td className={`px-1.5 py-1 text-right font-mono tabular-nums whitespace-nowrap ${getSuccessRateTextClass(model.success_rate)}`}>{formatUptimePct(model.success_rate)}</td>
              {/* 容器窄于 24rem 时把这 76px 让给模型名，成功率数值已承载同一信息 */}
              <td className='hidden px-1.5 py-1 align-middle @sm/perfcols:table-cell'><MiniSparkline series={model.series ?? []} /></td>
            </tr>
          ))}
        </tbody>
      </table>
      {foldable && (
        <button type='button' className='text-primary text-xs' onClick={(e) => { e.stopPropagation(); setExpanded(!expanded) }}>
          {expanded ? t('Collapse') : t('Expand all ({{count}})', { count: hidden })}
        </button>
      )}
    </div>
  )
}
