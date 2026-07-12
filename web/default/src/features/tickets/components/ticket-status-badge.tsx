import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/status-badge'
import {
  TICKET_STATUS_CONFIG,
  TICKET_PRIORITY_CONFIG,
  TICKET_TYPE_CONFIG,
} from '../constants'

export function TicketStatusBadge({ status }: { status: number }) {
  const { t } = useTranslation()
  const cfg = TICKET_STATUS_CONFIG[status]
  if (!cfg) return null
  return (
    <StatusBadge label={t(cfg.labelKey)} variant={cfg.variant} copyable={false} />
  )
}

export function TicketPriorityBadge({ priority }: { priority: number }) {
  const { t } = useTranslation()
  const cfg = TICKET_PRIORITY_CONFIG[priority]
  if (!cfg) return null
  return (
    <StatusBadge
      label={t(cfg.labelKey)}
      variant={cfg.variant}
      showDot={false}
      copyable={false}
    />
  )
}

export function TicketTypeBadge({ type }: { type: string }) {
  const { t } = useTranslation()
  const cfg = TICKET_TYPE_CONFIG[type]
  if (!cfg) return <span className="text-xs">{type}</span>
  return (
    <span className="text-muted-foreground text-xs">{t(cfg.labelKey)}</span>
  )
}
