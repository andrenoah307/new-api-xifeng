import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, FileText, ImageOff } from 'lucide-react'
import { toast } from 'sonner'
import { StatusBadge } from '@/components/status-badge'
import { getCommonHeaders } from '@/lib/api'
import { cn } from '@/lib/utils'
import { formatTimestampToDate } from '@/lib/format'
import { getAttachmentUrl, type TicketMessage, type TicketAttachment } from '../api'
import { roleBadgeVariant, roleBadgeLabel, humanFileSize } from '../constants'

interface TicketMessageItemProps {
  message: TicketMessage
  isMine: boolean
}

// 附件路由已从 cookie 鉴权（SessionAuth）改为 UserAuth（Authorization 头），
// 浏览器 <img src>/<a href> 无法携带请求头，统一走 fetch → blob URL。
async function fetchAttachmentBlobUrl(
  id: number,
  inline = false
): Promise<string> {
  const res = await fetch(getAttachmentUrl(id, inline), {
    headers: getCommonHeaders(),
  })
  if (!res.ok) throw new Error(res.statusText)
  return URL.createObjectURL(await res.blob())
}

async function downloadAttachment(a: TicketAttachment): Promise<void> {
  const url = await fetchAttachmentBlobUrl(a.id)
  const link = document.createElement('a')
  link.href = url
  link.download = a.file_name
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function AttachmentImage({ attachment }: { attachment: TicketAttachment }) {
  const { t } = useTranslation()
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let revoked = false
    let objectUrl: string | null = null
    fetchAttachmentBlobUrl(attachment.id, true)
      .then((url) => {
        if (revoked) {
          URL.revokeObjectURL(url)
          return
        }
        objectUrl = url
        setPreviewUrl(url)
      })
      .catch(() => {
        if (!revoked) setFailed(true)
      })
    return () => {
      revoked = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [attachment.id])

  return (
    <button
      type="button"
      onClick={() =>
        downloadAttachment(attachment).catch(() =>
          toast.error(t('Download failed'))
        )
      }
      className="block cursor-pointer overflow-hidden rounded-md border"
      title={attachment.file_name}
    >
      {previewUrl ? (
        <img
          src={previewUrl}
          alt={attachment.file_name}
          className="h-[120px] w-[120px] object-cover"
          loading="lazy"
        />
      ) : (
        <div className="bg-muted/50 flex h-[120px] w-[120px] items-center justify-center">
          {failed ? (
            <ImageOff className="text-muted-foreground h-5 w-5" />
          ) : (
            <span className="text-muted-foreground text-[10px]">
              {t('Loading...')}
            </span>
          )}
        </div>
      )}
    </button>
  )
}

function AttachmentImages({ items }: { items: TicketAttachment[] }) {
  if (items.length === 0) return null
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {items.map((a) => (
        <AttachmentImage key={a.id} attachment={a} />
      ))}
    </div>
  )
}

function AttachmentFiles({ items }: { items: TicketAttachment[] }) {
  const { t } = useTranslation()
  if (items.length === 0) return null
  return (
    <div className="mt-2 space-y-1">
      {items.map((a) => (
        <button
          key={a.id}
          type="button"
          onClick={() =>
            downloadAttachment(a).catch(() => toast.error(t('Download failed')))
          }
          className="bg-muted/50 hover:bg-muted flex w-full items-center gap-2 rounded-md px-3 py-2 text-xs transition-colors"
        >
          <FileText className="text-muted-foreground h-4 w-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate text-left">
            {a.file_name}
          </span>
          <span className="text-muted-foreground shrink-0">
            {humanFileSize(a.size)}
          </span>
          <Download className="text-muted-foreground h-3.5 w-3.5 shrink-0" />
        </button>
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
