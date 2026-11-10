import { Loader2, Paperclip, Send, X } from 'lucide-react'
import { useState, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useStatus } from '@/hooks/use-status'

import { humanFileSize } from '../constants'
import { useTicketAttachments } from '../hooks/use-ticket-attachments'

interface TicketReplyBoxProps {
  quickReplies?: string[]
  disabled?: boolean
  loading?: boolean
  onSubmit: (
    content: string,
    attachmentIds: number[]
  ) => Promise<boolean | void>
  placeholder?: string
}

export function TicketReplyBox({
  quickReplies = [],
  disabled,
  loading,
  onSubmit,
  placeholder,
}: TicketReplyBoxProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const attachmentEnabled = status?.ticket_attachment_enabled === true
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

  const submittingRef = useRef(false)
  const [busy, setBusy] = useState(false)

  const canSubmit =
    !disabled &&
    !loading &&
    !busy &&
    !uploading &&
    (content.trim().length > 0 || attachments.length > 0)

  const handleSubmit = useCallback(async () => {
    if (!canSubmit || submittingRef.current) return
    submittingRef.current = true
    setBusy(true)
    try {
      const sent = await onSubmit(content.trim(), attachmentIds)
      if (sent === false) return
      setContent('')
      reset()
    } catch {
      // The API mutation reports the error; preserve the draft for retry.
    } finally {
      submittingRef.current = false
      setBusy(false)
    }
  }, [canSubmit, content, attachmentIds, onSubmit, reset])

  const handleQuickReply = async (phrase: string) => {
    if (disabled || loading || uploading || submittingRef.current) return
    submittingRef.current = true
    setBusy(true)
    const started = Date.now()
    try {
      await onSubmit(phrase, [])
    } catch {
      // Leave the existing draft and attachments untouched.
    } finally {
      await new Promise((resolve) =>
        setTimeout(resolve, Math.max(0, 2000 - (Date.now() - started)))
      )
      submittingRef.current = false
      setBusy(false)
    }
  }

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files) handleFiles(e.target.files)
      e.target.value = ''
    },
    [handleFiles]
  )

  return (
    <div
      className='border-border rounded-lg border'
      onPasteCapture={
        attachmentEnabled && !disabled && !busy && !loading
          ? handlePaste
          : undefined
      }
    >
      {quickReplies.length > 0 && (
        <div
          className='flex max-h-48 flex-wrap gap-2 overflow-y-auto border-b p-3'
          aria-label={t('Quick replies')}
        >
          {quickReplies.map((phrase) => (
            <Button
              key={phrase}
              variant='outline'
              size='sm'
              className='h-auto max-w-full text-left break-words whitespace-normal'
              disabled={disabled || loading || busy || uploading}
              onClick={() => handleQuickReply(phrase)}
            >
              {phrase}
            </Button>
          ))}
        </div>
      )}
      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder={
          disabled
            ? t('Ticket is closed')
            : (placeholder ?? t('Type your reply...'))
        }
        disabled={disabled || loading || busy}
        maxLength={5000}
        className='min-h-[100px] resize-none border-0 focus-visible:ring-0'
      />
      {attachments.length > 0 && (
        <div className='border-border flex flex-wrap gap-2 border-t px-3 py-2'>
          {attachments.map((a) => (
            <div
              key={a.id}
              className='bg-muted flex items-center gap-1.5 rounded-md px-2 py-1 text-xs'
            >
              <Paperclip className='h-3 w-3' />
              <span className='max-w-[120px] truncate'>{a.file_name}</span>
              <span className='text-muted-foreground'>
                {humanFileSize(a.size)}
              </span>
              <button
                type='button'
                onClick={() => remove(a.id)}
                className='text-muted-foreground hover:text-foreground ml-1'
              >
                <X className='h-3 w-3' />
              </button>
            </div>
          ))}
        </div>
      )}
      <div className='border-border flex items-center justify-between border-t px-3 py-2'>
        <div>
          {attachmentEnabled && (
            <>
              <input
                type='file'
                multiple
                className='hidden'
                id='ticket-file-input'
                onChange={handleFileInput}
                disabled={disabled || busy || loading}
              />
              <Button
                variant='ghost'
                size='sm'
                disabled={disabled || uploading || busy || loading}
                render={
                  <label
                    htmlFor='ticket-file-input'
                    className='cursor-pointer'
                  />
                }
              >
                <Paperclip className='mr-1.5 h-4 w-4' />
                {uploading ? t('Uploading...') : t('Attach')}
              </Button>
            </>
          )}
        </div>
        <Button
          size='sm'
          disabled={!canSubmit || loading}
          onClick={handleSubmit}
        >
          {busy || loading ? (
            <Loader2 className='mr-1.5 h-4 w-4 animate-spin' />
          ) : (
            <Send className='mr-1.5 h-4 w-4' />
          )}
          {busy || loading ? t('Sending...') : t('Send')}
        </Button>
      </div>
    </div>
  )
}
