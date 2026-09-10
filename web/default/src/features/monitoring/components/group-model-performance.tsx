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
  // 选谁看后端的请求量倒序（topN = 最热的 N 个），怎么排看名称字典序。
  // 两件事分开：排序若下沉到后端，topN 会静默变成"字母序前 N 个"。
  // 必须先拷贝——models 是 props，就地排序会改调用方的数组。
  const visible = [...(foldable && !expanded ? models.slice(0, topN) : models)].sort((a, b) =>
    a.model_name.localeCompare(b.model_name, 'en', { sensitivity: 'base' })
  )
  const hidden = models.length - topN
  return (
    <div className='@container/perfcols space-y-2'>
      <div className='text-muted-foreground text-xs'>{t('Model performance metrics')} · {t('Last {{hours}} hours', { hours: windowHours })}</div>
      {/* 指标名只在表头出现一次：模型数达到 20 时，逐卡重复标签既撑不下也读不动 */}
      <table className='w-full border-collapse text-xs'>
        <thead>
          <tr className='text-muted-foreground border-border/60 border-b [&>th]:px-2 [&>th]:py-1 [&>th]:font-normal [&>th]:whitespace-nowrap'>
            <th scope='col' className='text-left'>{t('Model')}</th>
            <th scope='col' className='text-right' title={t('First token latency')}>{t('First token latency short')}</th>
            <th scope='col' className='text-right'>{t('Latency')}</th>
            <th scope='col' className='text-right' title={t('Throughput')}>{t('Throughput short')}</th>
            <th scope='col' className='text-right' title={t('Model success rate')}>{t('Model success rate short')}</th>
            {/* w-full 在 auto 布局下等价于"结算完各列固有宽度后，剩下的都归我"。
                它对固有宽度贡献为 0，所以下面单元格里还要有 min-w-* 兜底。 */}
            <th scope='col' className='hidden w-full text-left @lg/perfcols:table-cell'>{t('Trend')}</th>
          </tr>
        </thead>
        <tbody>
          {visible.map((model) => (
            <tr key={model.model_name} className='border-border/40 hover:bg-muted/40 border-b last:border-0'>
              {/* 名称列按内容定宽：生产最长模型名 35 字符约 200px，w-full 会让它
                  独吞整行剩余宽度，留下大片死白而数值列挤在最右。上限只用于兜住
                  异常长名，全名始终留在 title 里。 */}
              <td className='max-w-[260px] truncate px-2 py-1 font-medium' title={model.model_name}>{model.model_name}</td>
              <td className='px-2 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatLatency(model.avg_ttft_ms)}</td>
              <td className='px-2 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatLatency(model.avg_latency_ms)}</td>
              <td className='px-2 py-1 text-right font-mono tabular-nums whitespace-nowrap'>{formatThroughput(model.avg_tps)}</td>
              <td className={`px-2 py-1 text-right font-mono tabular-nums whitespace-nowrap ${getSuccessRateTextClass(model.success_rate)}`}>{formatUptimePct(model.success_rate)}</td>
              {/* auto 布局按单元格内容定列宽：色带的 w-full 与 flex-1 子项贡献为 0，
                  必须由这层 min-w-* 撑出列宽下限；上限交给表头的 w-full，
                  于是名称列让出的剩余宽度全部流进色带（24 段各自变宽）。
                  容器窄于 32rem 时整列让给模型名，成功率数值已承载同一信息。 */}
              <td className='hidden px-2 py-1 align-middle @lg/perfcols:table-cell'>
                <div className='min-w-32 @xl/perfcols:min-w-44 @3xl/perfcols:min-w-56'><MiniSparkline series={model.series ?? []} /></div>
              </td>
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
