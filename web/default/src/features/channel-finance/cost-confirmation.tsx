import { useQuery } from '@tanstack/react-query'
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
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

export interface FinanceRequest {
  request_key: string
  start: number
  end: number
  channel_id: number
  fallback_mode: 'none' | 'billed_basis'
}
interface CostPreview {
  fingerprint: string
  skipped: number
  channels: {
    channel_id: number
    name: string
    ratio: string
    peak?: { start: string; end: string; timezone: string; ratio: string }
  }[]
}

export function CostConfirmation(props: {
  request: FinanceRequest
  userId: number
  busy: boolean
  error: string
  onClose: () => void
  onConfirm: (fingerprint: string) => void
}) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(1)
  const preview = useQuery({
    queryKey: [
      'channel-finance',
      props.userId,
      100,
      'cost-preview',
      props.request.channel_id,
    ],
    queryFn: async ({ signal }) => {
      const result = await api.get('/api/channel-finance/cost-preview', {
        params: { channel_id: props.request.channel_id },
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      if (!result.data.success) throw new Error(t('Failed to load report'))
      return result.data.data as CostPreview
    },
    gcTime: 0,
  })
  const rows =
    preview.data?.channels.filter((row) =>
      `${row.channel_id} ${row.name}`
        .toLowerCase()
        .includes(filter.toLowerCase())
    ) ?? []
  const pages = Math.max(1, Math.ceil(rows.length / 20))
  const currentPage = Math.min(page, pages)
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.busy) props.onClose()
      }}
    >
      <DialogContent className='max-h-[90dvh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Confirm channel cost estimates')}</DialogTitle>
          <DialogDescription>
            {t(
              'Current channel cost settings apply to the entire report period. Channels without usable cost settings are skipped.'
            )}
          </DialogDescription>
        </DialogHeader>
        <p className='text-sm'>
          {new Date(props.request.start * 1000).toLocaleString()} -{' '}
          {new Date(props.request.end * 1000).toLocaleString()}
        </p>
        <Input
          aria-label={t('Filter channels')}
          placeholder={t('Filter channels')}
          value={filter}
          onChange={(event) => {
            setFilter(event.target.value)
            setPage(1)
          }}
        />
        {preview.isFetching && <p role='status'>{t('Loading...')}</p>}
        {(preview.isError || props.error) && (
          <p role='alert' className='text-destructive'>
            {props.error || t('Failed to load report')}
          </p>
        )}
        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead>
              <tr>
                <th className='p-2 text-left'>{t('Channel')}</th>
                <th className='p-2 text-right'>{t('Cost ratio')}</th>
              </tr>
            </thead>
            <tbody>
              {rows
                .slice((currentPage - 1) * 20, currentPage * 20)
                .map((row) => (
                  <tr key={row.channel_id} className='border-t'>
                    <td className='max-w-64 p-2 break-words'>
                      #{row.channel_id} {row.name}
                    </td>
                    <td className='p-2 text-right tabular-nums'>
                      {row.ratio}
                      {row.peak && (
                        <div className='text-muted-foreground text-xs'>
                          {row.peak.start}-{row.peak.end} ({row.peak.timezone})
                          × {row.peak.ratio}
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
        {preview.data && !preview.data.channels.length && (
          <p>{t('No channels have usable cost settings.')}</p>
        )}
        <div className='flex flex-wrap items-center justify-between gap-2 text-sm'>
          <span>
            {t('Total')}: {preview.data?.channels.length ?? 0} ·{' '}
            {t('Skipped channels: {{count}}', {
              count: preview.data?.skipped ?? 0,
            })}
          </span>
          <div className='flex items-center gap-2'>
            <Button
              title={t('Previous')}
              aria-label={t('Previous')}
              size='icon'
              variant='outline'
              disabled={currentPage <= 1}
              onClick={() => setPage(currentPage - 1)}
            >
              <ChevronLeft />
            </Button>
            <span>
              {currentPage}/{pages}
            </span>
            <Button
              title={t('Next')}
              aria-label={t('Next')}
              size='icon'
              variant='outline'
              disabled={currentPage >= pages}
              onClick={() => setPage(currentPage + 1)}
            >
              <ChevronRight />
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button
            title={t('Refresh')}
            aria-label={t('Refresh')}
            size='icon'
            variant='outline'
            disabled={props.busy || preview.isFetching}
            onClick={() => void preview.refetch()}
          >
            <RefreshCw />
          </Button>
          <Button
            variant='outline'
            disabled={props.busy}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={
              props.busy ||
              preview.isFetching ||
              preview.isError ||
              !preview.data?.channels.length
            }
            onClick={() => {
              if (preview.data) props.onConfirm(preview.data.fingerprint)
            }}
          >
            {t('Confirm and generate')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
