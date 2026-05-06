import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getMyCommissionRecords } from '../api'

export function CommissionRecordsTable() {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [page, setPage] = useState(1)
  const pageSize = 10

  const { data, isLoading } = useQuery({
    queryKey: ['commission_records', page, pageSize],
    queryFn: () => getMyCommissionRecords(page, pageSize),
    enabled: expanded,
  })

  const records = data?.data?.records ?? []
  const total = data?.data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  if (!expanded) {
    return (
      <Button
        variant='ghost'
        size='sm'
        className='text-muted-foreground w-full'
        onClick={() => setExpanded(true)}
      >
        <ChevronDown className='mr-1 size-4' />
        {t('View Commission Records')}
      </Button>
    )
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between'>
        <h4 className='text-sm font-medium'>{t('Commission Records')}</h4>
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground h-7'
          onClick={() => setExpanded(false)}
        >
          <ChevronUp className='mr-1 size-3' />
          {t('Collapse')}
        </Button>
      </div>

      {isLoading ? (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('Loading...')}
        </div>
      ) : records.length === 0 ? (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('No commission records')}
        </div>
      ) : (
        <>
          <div className='rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Time')}</TableHead>
                  <TableHead>{t('Top-Up Amount')}</TableHead>
                  <TableHead>{t('Rate')}</TableHead>
                  <TableHead>{t('Commission')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className='text-muted-foreground text-xs'>
                      {formatTimestampToDate(r.created_at)}
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      ${r.topup_money.toFixed(2)}
                    </TableCell>
                    <TableCell className='text-xs'>
                      {r.commission_rate}%
                    </TableCell>
                    <TableCell className='font-mono text-xs font-medium text-green-600 dark:text-green-400'>
                      +{formatQuota(r.commission_quota)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={r.is_manual ? 'secondary' : 'outline'}>
                        {r.is_manual ? t('Manual') : t('Online')}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {totalPages > 1 && (
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs'>
                {t('Total')}: {total}
              </span>
              <div className='flex gap-1'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
