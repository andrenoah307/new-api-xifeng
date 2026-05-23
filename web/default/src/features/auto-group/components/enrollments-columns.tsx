import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Trash2, MoreHorizontal as DotsHorizontalIcon } from 'lucide-react'
import { formatTimestampToDate } from '@/lib/format'
import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { AutoGroupEnrollment } from '../types'
import { useAutoGroup } from './auto-group-provider'

function EnrollmentRowActions({
  enrollment,
}: {
  enrollment: AutoGroupEnrollment
}) {
  const { t } = useTranslation()
  const { setOpen, setCurrentEnrollment } = useAutoGroup()

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            className='data-popup-open:bg-muted flex h-8 w-8 p-0'
          />
        }
      >
        <DotsHorizontalIcon className='h-4 w-4' />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[160px]'>
        <DropdownMenuItem
          onClick={() => {
            setCurrentEnrollment(enrollment)
            setOpen('unenroll')
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Unenroll')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function useEnrollmentsColumns(): ColumnDef<AutoGroupEnrollment>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('ID')} />
      ),
      cell: ({ row }) => (
        <div className='w-[60px]'>{row.getValue('id')}</div>
      ),
    },
    {
      accessorKey: 'username',
      meta: { label: t('User'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: ({ row }) => {
        const username = row.original.username
        const displayName = row.original.display_name
        return (
          <div className='max-w-[200px]'>
            <div className='truncate font-medium'>{username}</div>
            {displayName && displayName !== username && (
              <div className='text-muted-foreground truncate text-xs'>
                {displayName}
              </div>
            )}
          </div>
        )
      },
    },
    {
      accessorKey: 'original_group',
      meta: { label: t('Original Group') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Original Group')} />
      ),
      cell: ({ row }) => (
        <Badge variant='outline'>
          {row.getValue('original_group') || '-'}
        </Badge>
      ),
    },
    {
      accessorKey: 'current_group',
      meta: { label: t('Current Group') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Current Group')} />
      ),
      cell: ({ row }) => (
        <Badge variant='secondary'>
          {row.getValue('current_group') || '-'}
        </Badge>
      ),
    },
    {
      accessorKey: 'current_rule_id',
      meta: { label: t('Rule ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Rule ID')} />
      ),
      cell: ({ row }) => <div>{row.getValue('current_rule_id')}</div>,
    },
    {
      accessorKey: 'enrolled_at',
      meta: { label: t('Enrolled At'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Enrolled At')} />
      ),
      cell: ({ row }) => {
        const time = row.getValue('enrolled_at') as number
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {time > 0 ? formatTimestampToDate(time) : '-'}
          </div>
        )
      },
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <EnrollmentRowActions enrollment={row.original} />
      ),
    },
  ]
}
