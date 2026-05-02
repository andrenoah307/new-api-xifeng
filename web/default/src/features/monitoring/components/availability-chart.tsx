import { useMemo, memo } from 'react'
import { useTranslation } from 'react-i18next'
import { VChart } from '@visactor/react-vchart'
import type { MonitoringHistoryPoint } from '../api'
import { alignAndFillHistory } from '../constants'

interface AvailabilityCacheChartProps {
  history: MonitoringHistoryPoint[]
  intervalMinutes?: number
  compact?: boolean
}

const VCHART_OPTION = { mode: 'desktop-browser' as const }
const tooltipValueFn = (datum: { value: number }) => `${datum.value.toFixed(1)}%`
const formatPercent = (v: number) => `${v}%`

const AvailabilityCacheChart = memo(function AvailabilityCacheChart({
  history,
  intervalMinutes = 5,
  compact = false,
}: AvailabilityCacheChartProps) {
  const { t } = useTranslation()

  const chartData = useMemo(
    () => alignAndFillHistory(history, intervalMinutes),
    [history, intervalMinutes]
  )

  const yMin = useMemo(() => {
    if (!chartData || chartData.length === 0) return 0
    const vals = chartData.map((d) => d.value).filter((v) => v > 0)
    if (vals.length === 0) return 0
    const min = Math.min(...vals)
    return Math.max(0, Math.floor(min / 5) * 5 - 5)
  }, [chartData])

  const h = compact ? 120 : 260

  const spec = useMemo(() => {
    if (!chartData || chartData.length === 0) return null

    const tooltipKeyFn = (datum: { type: string }) =>
      datum.type === 'availability' ? t('Availability') : t('Cache Hit Rate')

    return {
      type: 'line' as const,
      data: [{ id: 'history', values: chartData }],
      xField: 'time',
      yField: 'value',
      seriesField: 'type',
      height: h,
      padding: compact
        ? { top: 4, bottom: 20, left: 4, right: 4 }
        : { top: 12, bottom: 24, left: 8, right: 8 },
      animation: compact ? false : undefined,
      line: {
        style: {
          lineWidth: compact ? 1.5 : 2,
          curveType: 'monotone',
        },
      },
      point: { visible: false },
      axes: [
        {
          orient: 'bottom',
          label: {
            autoRotate: true,
            autoHide: true,
            style: { fontSize: compact ? 9 : 11 },
          },
        },
        {
          orient: 'left',
          min: yMin,
          max: 100,
          label: compact
            ? { visible: false }
            : {
                formatMethod: formatPercent,
                style: { fontSize: 11 },
              },
        },
      ],
      legends: compact
        ? { visible: false }
        : {
            visible: true,
            orient: 'top' as const,
            position: 'start' as const,
            data: [
              { label: t('Availability'), shape: { fill: '#3b82f6' } },
              { label: t('Cache Hit Rate'), shape: { fill: '#22c55e' } },
            ],
          },
      tooltip: {
        mark: {
          content: [{ key: tooltipKeyFn, value: tooltipValueFn }],
        },
        dimension: {
          content: [{ key: tooltipKeyFn, value: tooltipValueFn }],
        },
      },
      color: ['#3b82f6', '#22c55e'],
      crosshair: {
        xField: { visible: true, line: { type: 'line' } },
      },
    }
  }, [chartData, yMin, compact, t, h])

  if (!spec) {
    return (
      <div
        className="text-muted-foreground flex items-center justify-center"
        style={{ height: h, fontSize: compact ? 12 : 14 }}
      >
        {t('No history data')}
      </div>
    )
  }

  return <VChart spec={spec} option={VCHART_OPTION} skipFunctionDiff />
})

export default AvailabilityCacheChart
