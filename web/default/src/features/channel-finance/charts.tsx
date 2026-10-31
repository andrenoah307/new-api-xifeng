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
import { VChart } from '@visactor/react-vchart'
import type { ISpec } from '@visactor/vchart'
import { useTranslation } from 'react-i18next'
import { useTheme } from '@/context/theme-provider'
import { VCHART_OPTION } from '@/lib/vchart'
import { formatReportAmount } from './format'
import type { FinanceRow } from './types'

const metrics = [
  ['known_revenue', 'Consumption by channel'],
  ['cost', 'Cost by channel'],
  ['known_profit', 'Profit by channel'],
] as const
const colors = [
  '#2563eb',
  '#059669',
  '#d97706',
  '#db2777',
  '#0891b2',
  '#7c3aed',
  '#65a30d',
  '#dc2626',
]

export function FinanceCharts({ rows }: { rows: FinanceRow[] }) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  return (
    <div className='grid min-w-0 gap-6 border-b py-5 xl:grid-cols-3'>
      {metrics.map(([metric, title]) => {
        const data = rows
          .map((row) => ({
            channel: `#${row.channel_id}`,
            name: `${row.channel_name || '-'} #${row.channel_id}`,
            value: Number(row[metric]),
            amount: formatReportAmount(row[metric], metric),
          }))
          .filter((item) => Number.isFinite(item.value))
          .sort((a, b) => b.value - a.value)
        const profit = metric === 'known_profit'
        const hasData = data.some((item) => item.value !== 0)
        const spec: ISpec = {
          data: [{ id: 'channels', values: data }],
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
          background: 'transparent',
          color: colors,
          animation: false,
          seriesField: 'channel',
          ...(profit
            ? {
                type: 'bar',
                direction: 'horizontal',
                xField: 'value',
                yField: 'channel',
                axes: [
                  { orient: 'left', type: 'band' },
                  { orient: 'bottom', type: 'linear', zero: true },
                ],
              }
            : {
                type: 'pie',
                valueField: 'value',
                categoryField: 'channel',
                outerRadius: 0.85,
                innerRadius: 0.6,
                label: { visible: false },
              }),
          legends: { visible: false },
          tooltip: {
            mark: {
              title: { value: (datum) => datum?.name },
              content: [
                { key: () => t(title), value: (datum) => datum?.amount },
              ],
            },
          },
        }
        return (
          <section key={metric} aria-label={t(title)} className='min-w-0'>
            <h3 className='text-sm font-semibold'>{t(title)}</h3>
            <div className='h-64 min-w-0 overflow-y-auto'>
              {hasData ? (
                <div
                  style={{
                    height: profit ? Math.max(256, data.length * 26 + 40) : 256,
                  }}
                >
                  <VChart spec={spec} options={VCHART_OPTION} />
                </div>
              ) : (
                <p className='py-24 text-center text-sm text-muted-foreground'>
                  {t('No data')}
                </p>
              )}
            </div>
            <ul className='max-h-40 space-y-2 overflow-y-auto text-xs'>
              {data.map((item, index) => (
                <li key={item.channel} className='flex items-start gap-2'>
                  <span
                    aria-hidden='true'
                    className='mt-1 size-2 shrink-0 rounded-full'
                    style={{ backgroundColor: colors[index % colors.length] }}
                  />
                  <span className='min-w-0 flex-1 break-all'>{item.name}</span>
                  <span className='shrink-0 tabular-nums'>{item.amount}</span>
                </li>
              ))}
            </ul>
          </section>
        )
      })}
    </div>
  )
}
