import { useTranslation } from 'react-i18next'

import { CHANNEL_STATUS } from '../constants'

export function ChannelCacheRate(props: {
  status?: number
  rate?: number | null
}) {
  const { t } = useTranslation()
  if (props.status !== CHANNEL_STATUS.ENABLED) return null

  return (
    <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 text-xs'>
      <span>
        {t('Cache')} · {t('Last hour')}
      </span>
      <span className='text-foreground font-mono tabular-nums'>
        {props.rate != null && Number.isFinite(props.rate) && props.rate >= 0
          ? `${props.rate.toFixed(1)}%`
          : '—'}
      </span>
    </div>
  )
}
