import { useTranslation } from 'react-i18next'
import { Download, FileText } from 'lucide-react'
import { StatusBadge } from '@/components/status-badge'
import { cn } from '@/lib/utils'
import { formatTimestampToDate } from '@/lib/format'
import type { TicketMessage, TicketAttachment } from '../api'
import { getAttachmentUrl } from '../api'
import { roleBadgeVariant, roleBadgeLabel, humanFileSize } from '../constants'

interface TicketMessageItemProps {
  message: TicketMessage
  isMine: boolean
}

function AttachmentImages({ items }: { items: TicketAttachment[] }) {
  if (items.length === 0) return null
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {items.map((a) => (
        <a
          key={a.id}
          href={getAttachmentUrl(a.id)}
          target="_blank"
          rel="noopener noreferrer"
          className="block overflow-hidden rounded-md border"
        >
          <img
            src={getAttachmentUrl(a.id, true)}
            alt={a.file_name}
            className="h-[120px] w-[120px] object-cover"
            loading="lazy"
          />
        </a>
      ))}
    </div>
  )
}

function AttachmentFiles({ items }: { items: TicketAttachment[] }) {
  if (items.length === 0) return null
  return (
    <div className="mt-2 space-y-1">
      {items.map((a) => (
        <a
          key={a.id}
          href={getAttachmentUrl(a.id)}
          download={a.file_name}
          className="bg-muted/50 hover:bg-muted flex items-center gap-2 rounded-md px-3 py-2 text-xs transition-colors"
        >
          <FileText className="text-muted-foreground h-4 w-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{a.file_name}</span>
          <span className="text-muted-foreground shrink-0">
            {humanFileSize(a.size)}
          </span>
          <Download className="text-muted-foreground h-3.5 w-3.5 shrink-0" />
        </a>
      ))}
    </div>
  )
}

export function TicketMessageItem({ message, isMine }: TicketMessageItemProps) {
  const { t } = useTranslation()
  const attachments = message.attachments ?? []
  const images = attachments.filter((a) => a.mime_type?.startsWith('image/'))
  const files = attachments.filter((a) => !a.mime_type?.startsWith('image/'))

  return (
    <div
      className={cn('flex', isMine ? 'justify-end' : 'justify-start')}
    >
      <div
        className={cn(
          'max-w-[78%] rounded-xl px-4 py-3',
          isMine
            ? 'bg-primary/10 text-foreground'
            : 'bg-muted text-foreground'
        )}
      >
        <div className="mb-1 flex items-center gap-2">
          <span className="text-xs font-medium">
            {message.username || `#${message.user_id}`}
          </span>
          <StatusBadge
            label={t(roleBadgeLabel(message.role))}
            variant={roleBadgeVariant(message.role)}
            size="sm"
            showDot={false}
            copyable={false}
          />
          <span className="text-muted-foreground text-[10px]">
            {formatTimestampToDate(message.created_time)}
          </span>
        </div>
        {message.content && (
          <p className="whitespace-pre-wrap text-sm leading-relaxed">
            {message.content}
          </p>
        )}
        <AttachmentImages items={images} />
        <AttachmentFiles items={files} />
      </div>
    </div>
  )
}
