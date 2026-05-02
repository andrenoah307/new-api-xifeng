import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Paperclip, Send, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useTicketAttachments } from '../hooks/use-ticket-attachments'
import { humanFileSize } from '../constants'

interface TicketReplyBoxProps {
  disabled?: boolean
  loading?: boolean
  onSubmit: (content: string, attachmentIds: number[]) => Promise<void>
  placeholder?: string
}

export function TicketReplyBox({
  disabled,
  loading,
  onSubmit,
  placeholder,
}: TicketReplyBoxProps) {
  const { t } = useTranslation()
  const [content, setContent] = useState('')
  const {
    attachments,
    uploading,
    attachmentIds,
    handleFiles,
    handlePaste,
    remove,
    reset,
  } = useTicketAttachments()

  const canSubmit =
    !disabled && !loading && (content.trim().length > 0 || attachments.length > 0)

  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return
    await onSubmit(content.trim(), attachmentIds)
    setContent('')
    reset()
  }, [canSubmit, content, attachmentIds, onSubmit, reset])

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files) handleFiles(e.target.files)
      e.target.value = ''
    },
    [handleFiles]
  )

  return (
    <div className="border-border rounded-lg border" onPasteCapture={handlePaste}>
      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder={
          disabled
            ? t('Ticket is closed')
            : placeholder ?? t('Type your reply...')
        }
        disabled={disabled}
        maxLength={5000}
        className="min-h-[100px] resize-none border-0 focus-visible:ring-0"
      />
      {attachments.length > 0 && (
        <div className="border-border flex flex-wrap gap-2 border-t px-3 py-2">
          {attachments.map((a) => (
            <div
              key={a.id}
              className="bg-muted flex items-center gap-1.5 rounded-md px-2 py-1 text-xs"
            >
              <Paperclip className="h-3 w-3" />
              <span className="max-w-[120px] truncate">{a.file_name}</span>
              <span className="text-muted-foreground">
                {humanFileSize(a.size)}
              </span>
              <button
                type="button"
                onClick={() => remove(a.id)}
                className="text-muted-foreground hover:text-foreground ml-1"
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="border-border flex items-center justify-between border-t px-3 py-2">
        <div>
          <input
            type="file"
            multiple
            className="hidden"
            id="ticket-file-input"
            onChange={handleFileInput}
            disabled={disabled}
          />
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || uploading}
            asChild
          >
            <label htmlFor="ticket-file-input" className="cursor-pointer">
              <Paperclip className="mr-1.5 h-4 w-4" />
              {uploading ? t('Uploading...') : t('Attach')}
            </label>
          </Button>
        </div>
        <Button
          size="sm"
          disabled={!canSubmit || loading}
          onClick={handleSubmit}
        >
          <Send className="mr-1.5 h-4 w-4" />
          {loading ? t('Sending...') : t('Send')}
        </Button>
      </div>
    </div>
  )
}
