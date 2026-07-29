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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { toast } from 'sonner'
import { Download, ImageOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTablePagination } from '@/components/data-table/core/pagination'
import { api, getCommonHeaders } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'
import { useLogsViewScope } from './usage-logs-provider'

interface ImageResultFile {
  idx: number
  mime: string
  size: number
  source: string
}

interface ImageResultRecord {
  id: number
  user_id: number
  username: string
  model_name: string
  mode: string
  prompt: string
  request_id: string
  created_time: number
  files: ImageResultFile[]
}

interface ImageResultListResponse {
  items: ImageResultRecord[] | null
  total: number
}

const PAGE_SIZE = 10

function getImageResultFileUrl(
  recordId: number,
  idx: number,
  isAdminView: boolean
): string {
  return isAdminView
    ? `/api/image_result/${recordId}/file/${idx}`
    : `/api/image_result/self/${recordId}/file/${idx}`
}

async function fetchImageResults(
  page: number,
  isAdminView: boolean
): Promise<ImageResultListResponse> {
  const path = isAdminView ? '/api/image_result/' : '/api/image_result/self'
  const res = await api.get(path, {
    params: { p: page, page_size: PAGE_SIZE },
  })
  const data = res.data?.data
  return { items: data?.items ?? [], total: data?.total ?? 0 }
}

// 下载接口走 UserAuth（Authorization 头），<img src> 带不了请求头，
// 与工单附件一致：fetch → blob URL；blob URL 由调用方负责 revoke。
function useImageResultBlobUrl(
  record: ImageResultRecord,
  file: ImageResultFile,
  isAdminView: boolean
) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let revoked = false
    let objectUrl: string | null = null
    setBlobUrl(null)
    setFailed(false)
    fetch(getImageResultFileUrl(record.id, file.idx, isAdminView), {
      headers: getCommonHeaders(),
    })
      .then((res) => {
        if (!res.ok) throw new Error(res.statusText)
        return res.blob()
      })
      .then((blob) => {
        const url = URL.createObjectURL(blob)
        if (revoked) {
          URL.revokeObjectURL(url)
          return
        }
        objectUrl = url
        setBlobUrl(url)
      })
      .catch(() => {
        if (!revoked) setFailed(true)
      })
    return () => {
      revoked = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [record.id, file.idx, isAdminView])

  return { blobUrl, failed }
}

function downloadFileName(
  record: ImageResultRecord,
  file: ImageResultFile
): string {
  const ext = file.mime.split('/')[1] || 'png'
  return `${record.model_name || 'image'}-${record.id}-${file.idx}.${ext}`
}

function triggerDownload(blobUrl: string, fileName: string) {
  const a = document.createElement('a')
  a.href = blobUrl
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  a.remove()
}

function ImageResultThumb({
  record,
  file,
  isAdminView,
  onPreview,
}: {
  record: ImageResultRecord
  file: ImageResultFile
  isAdminView: boolean
  onPreview: (record: ImageResultRecord, file: ImageResultFile) => void
}) {
  const { t } = useTranslation()
  const { blobUrl, failed } = useImageResultBlobUrl(record, file, isAdminView)

  if (failed) {
    return (
      <div className='flex h-16 w-16 items-center justify-center rounded border bg-muted'>
        <ImageOff className='h-4 w-4 text-muted-foreground' />
      </div>
    )
  }
  if (!blobUrl) {
    return <div className='h-16 w-16 animate-pulse rounded border bg-muted' />
  }
  return (
    <button
      type='button'
      onClick={() => onPreview(record, file)}
      title={t('Click to preview')}
      className='block'
    >
      <img
        src={blobUrl}
        alt={record.prompt || record.model_name}
        className='h-16 w-16 rounded border object-cover transition-opacity hover:opacity-80'
      />
    </button>
  )
}

// 预览弹窗：大图 + 下载按钮。弹窗内独立 fetch 原图（缩略图与弹窗生命周期不同，
// 各自持有/释放自己的 blob URL，互不影响）。
function ImageResultPreviewDialog({
  target,
  isAdminView,
  open,
  onOpenChange,
}: {
  target: { record: ImageResultRecord; file: ImageResultFile } | null
  isAdminView: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  if (!target) return null
  return (
    <PreviewDialogContent
      record={target.record}
      file={target.file}
      isAdminView={isAdminView}
      open={open}
      onOpenChange={onOpenChange}
    />
  )
}

function PreviewDialogContent({
  record,
  file,
  isAdminView,
  open,
  onOpenChange,
}: {
  record: ImageResultRecord
  file: ImageResultFile
  isAdminView: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const { blobUrl, failed } = useImageResultBlobUrl(record, file, isAdminView)

  const handleDownload = () => {
    if (!blobUrl) {
      toast.error(t('Download failed'))
      return
    }
    triggerDownload(blobUrl, downloadFileName(record, file))
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Image Preview')}
      description={`${record.model_name} · ${formatTimestamp(record.created_time)}`}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <Button onClick={handleDownload} disabled={!blobUrl}>
          <Download className='mr-2 h-4 w-4' />
          {t('Download')}
        </Button>
      }
    >
      <div className='bg-muted/50 relative flex min-h-[300px] items-center justify-center rounded-lg border'>
        {failed ? (
          <p className='text-muted-foreground text-sm'>
            {t('Failed to load image')}
          </p>
        ) : !blobUrl ? (
          <div className='h-[300px] w-full animate-pulse rounded-lg bg-muted' />
        ) : (
          <img
            src={blobUrl}
            alt={record.prompt || record.model_name}
            className='max-h-[550px] w-full rounded-lg object-contain'
          />
        )}
      </div>
      {record.prompt && (
        <div className='bg-muted rounded-md p-3'>
          <p className='text-muted-foreground text-xs break-all'>
            {record.prompt}
          </p>
        </div>
      )}
    </Dialog>
  )
}

export function ImageResultsTab() {
  const { t } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const [page, setPage] = useState(1)
  const [previewTarget, setPreviewTarget] = useState<{
    record: ImageResultRecord
    file: ImageResultFile
  } | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)

  // 视角切换（全部/仅自己）后回到第一页，避免页码越界
  useEffect(() => {
    setPage(1)
  }, [isAdminView])

  const { data, isLoading } = useQuery({
    queryKey: ['image-results', isAdminView, page],
    queryFn: () => fetchImageResults(page, isAdminView),
    placeholderData: (prev) => prev,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0

  const table = useReactTable({
    data: items,
    columns: [],
    pageCount: Math.max(1, Math.ceil(total / PAGE_SIZE)),
    state: { pagination: { pageIndex: page - 1, pageSize: PAGE_SIZE } },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: page - 1, pageSize: PAGE_SIZE })
          : updater
      setPage(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const handlePreview = (
    record: ImageResultRecord,
    file: ImageResultFile
  ) => {
    setPreviewTarget({ record, file })
    setPreviewOpen(true)
  }

  const colSpan = isAdminView ? 6 : 5

  return (
    <div className='space-y-3'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Successful image generations are saved here for a limited time. If a request timed out on the client (e.g. CDN 120s limit), you can still download the result below.'
        )}
      </p>
      <div className='rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              {isAdminView && <TableHead>{t('Username')}</TableHead>}
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Prompt')}</TableHead>
              <TableHead>{t('Images')}</TableHead>
              <TableHead>{t('Request ID')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={colSpan} className='py-8 text-center'>
                  {t('Loading...')}
                </TableCell>
              </TableRow>
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={colSpan}
                  className='py-8 text-center text-muted-foreground'
                >
                  {t('No data')}
                </TableCell>
              </TableRow>
            ) : (
              items.map((record) => (
                <TableRow key={record.id}>
                  <TableCell className='whitespace-nowrap text-xs'>
                    {formatTimestamp(record.created_time)}
                  </TableCell>
                  {isAdminView && (
                    <TableCell className='text-xs'>
                      {record.username || record.user_id}
                    </TableCell>
                  )}
                  <TableCell className='text-xs'>
                    {record.model_name}
                    <span className='text-muted-foreground ml-1'>
                      ({record.mode})
                    </span>
                  </TableCell>
                  <TableCell
                    className='max-w-[240px] truncate text-xs'
                    title={record.prompt}
                  >
                    {record.prompt || '-'}
                  </TableCell>
                  <TableCell>
                    <div className='flex flex-wrap gap-1'>
                      {(record.files ?? []).map((file) => (
                        <ImageResultThumb
                          key={file.idx}
                          record={record}
                          file={file}
                          isAdminView={isAdminView}
                          onPreview={handlePreview}
                        />
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {record.request_id || '-'}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} />

      <ImageResultPreviewDialog
        target={previewTarget}
        isAdminView={isAdminView}
        open={previewOpen}
        onOpenChange={setPreviewOpen}
      />
    </div>
  )
}
