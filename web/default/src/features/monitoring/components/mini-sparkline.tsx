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
import { useTranslation } from 'react-i18next'
import { formatFRT, segmentColor, segmentLabel } from '../constants'

export default function MiniSparkline({
  series,
  ttftSeries,
  fallbackTtftMs,
}: {
  series: (number | null)[]
  ttftSeries?: (number | null)[]
  fallbackTtftMs?: number | null
}) {
  const { t } = useTranslation()
  if (!series.length) return null
  return <div className='flex h-2 w-full flex-nowrap gap-px overflow-hidden' aria-label={t('Model success rate')}>
    {series.map((value, index) => {
      // 非流式请求不产生首字样本（hasTtft := info.IsStream && info.HasSendResponse()），
      // 所以有请求的槽也可能 ttft_count == 0，此时用整窗均值总比无色好。
      const frt = ttftSeries?.[index] ?? fallbackTtftMs ?? null
      const title = value == null ? t('No data available') : `${segmentLabel(value, frt, t)} · ${value}%${frt != null ? ` · ${formatFRT(frt)}` : ''}`
      return <span key={index} className='min-w-0 flex-1' style={{ background: segmentColor(value, frt) }} title={title} /> // eslint-disable-line react/no-array-index-key -- Time slots retain their positions, including empty slots.
    })}
  </div>
}
