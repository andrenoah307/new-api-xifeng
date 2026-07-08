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
  // 进行中的上传数：state 在同步循环内不会更新，靠它保证并发上传不越过 MAX_COUNT
  const pendingRef = useRef(0)

  const validate = useCallback(
    (file: File): boolean => {
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
    [t]
  )

  const upload = useCallback(
    async (file: File) => {
      if (attachments.length + pendingRef.current >= MAX_COUNT) {
        toast.error(t('Maximum {{count}} files allowed', { count: MAX_COUNT }))
        return
      }
      if (!validate(file)) return
      pendingRef.current++
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
        pendingRef.current--
        setUploading(pendingRef.current > 0)
      }
    },
    [attachments.length, validate, t]
  )

  const remove = useCallback(
    async (id: number) => {
      const ok = await deleteAttachment(id).catch(() => false)
      // 删除失败时保留 UI 条目，避免服务端残留而界面已消失（拦截器已提示错误）
      if (!ok) return
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
      const list = Array.from(files)
      const allowed = Math.max(
        0,
        MAX_COUNT - attachments.length - pendingRef.current
      )
      if (list.length > allowed) {
        toast.error(t('Maximum {{count}} files allowed', { count: MAX_COUNT }))
      }
      list.slice(0, allowed).forEach((f) => upload(f))
    },
    [upload, attachments.length, t]
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
