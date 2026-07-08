import { useState, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  uploadAttachment,
  deleteAttachment,
  type TicketAttachment,
} from '../api'

const MAX_SIZE = 50 * 1024 * 1024
const MAX_COUNT = 5
const BLOCKED_EXTS = ['svg', 'exe', 'bat', 'cmd', 'sh', 'ps1']
const ALLOWED_EXTS = [
  'jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp',
  'json', 'xml', 'txt', 'log', 'md', 'csv', 'pdf',
]

interface UploadedFile extends TicketAttachment {
  uid: string
}

export function useTicketAttachments() {
  const { t } = useTranslation()
  const [attachments, setAttachments] = useState<UploadedFile[]>([])
  const [uploading, setUploading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const validate = useCallback(
    (file: File): boolean => {
      if (attachments.length >= MAX_COUNT) {
        toast.error(t('Maximum {{count}} files allowed', { count: MAX_COUNT }))
        return false
      }
      if (file.size > MAX_SIZE) {
        toast.error(t('File size exceeds {{size}} MB limit', { size: 50 }))
        return false
      }
      const ext = file.name.split('.').pop()?.toLowerCase() ?? ''
      if (BLOCKED_EXTS.includes(ext)) {
        toast.error(t('File type not allowed'))
        return false
      }
      if (ext && !ALLOWED_EXTS.includes(ext)) {
        toast.error(t('File type not allowed'))
        return false
      }
      return true
    },
    [attachments.length, t]
  )

  const upload = useCallback(
    async (file: File) => {
      if (!validate(file)) return
      setUploading(true)
      try {
        const result = await uploadAttachment(file)
        if (result) {
          setAttachments((prev) => [
            ...prev,
            { ...result, uid: `${Date.now()}-${Math.random().toString(36).slice(2)}` },
          ])
        }
      } catch {
        toast.error(t('Upload failed'))
      } finally {
        setUploading(false)
      }
    },
    [validate, t]
  )

  const remove = useCallback(
    async (id: number) => {
      await deleteAttachment(id)
      setAttachments((prev) => prev.filter((a) => a.id !== id))
    },
    []
  )

  const reset = useCallback(() => {
    setAttachments([])
  }, [])

  const discardAll = useCallback(async () => {
    for (const a of attachments) {
      try {
        await deleteAttachment(a.id)
      } catch {
        // ignore cleanup errors
      }
    }
    setAttachments([])
  }, [attachments])

  const handleFiles = useCallback(
    (files: FileList | File[]) => {
      Array.from(files).forEach((f) => upload(f))
    },
    [upload]
  )

  const handlePaste = useCallback(
    (e: React.ClipboardEvent) => {
      const items = e.clipboardData?.items
      if (!items) return
      const files: File[] = []
      for (const item of Array.from(items)) {
        if (item.kind === 'file') {
          const file = item.getAsFile()
          if (file) files.push(file)
        }
      }
      if (files.length > 0) {
        e.preventDefault()
        handleFiles(files)
      }
    },
    [handleFiles]
  )

  const attachmentIds = attachments.map((a) => a.id)

  return {
    attachments,
    uploading,
    attachmentIds,
    inputRef,
    upload,
    remove,
    reset,
    discardAll,
    handleFiles,
    handlePaste,
  }
}
