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
import { ImageOff } from 'lucide-react'
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
// 与工单附件一致：fetch → blob URL 展示，卸载时 revoke。
function ImageResultThumb({
  record,
  file,
  isAdminView,
}: {
  record: ImageResultRecord
  file: ImageResultFile
  isAdminView: boolean
}) {
  const { t } = useTranslation()
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let revoked = false
    let objectUrl: string | null = null
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

  const handleDownload = () => {
    if (!blobUrl) {
      toast.error(t('Download failed'))
      return
    }
    const ext = file.mime.split('/')[1] || 'png'
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = `${record.model_name || 'image'}-${record.id}-${file.idx}.${ext}`
    document.body.appendChild(a)
    a.click()
    a.remove()
  }

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
      onClick={handleDownload}
      title={t('Click to download')}
      className='block'
    >
      <img
        src={blobUrl}
        alt={record.prompt || record.model_name}
        className='h-16 w-16 rounded border object-cover'
      />
    </button>
  )
}

export function ImageResultsTab() {
  const { t } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const [page, setPage] = useState(1)

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
              {isAdminView && <TableHead>{t('User ID')}</TableHead>}
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
                    <TableCell className='text-xs'>{record.user_id}</TableCell>
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
    </div>
  )
}
