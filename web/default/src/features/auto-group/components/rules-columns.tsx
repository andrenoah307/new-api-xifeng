import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Edit,
  Trash2,
  MoreHorizontal as DotsHorizontalIcon,
} from 'lucide-react'
import { formatTimestampToDate } from '@/lib/format'
import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { GroupBadge } from '@/components/group-badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { updateRule } from '../api'
import { METRICS_MAP, SUCCESS_MESSAGES } from '../constants'
import type { AutoGroupRule, AutoGroupCondition } from '../types'
import { useAutoGroup } from './auto-group-provider'

function ConditionsSummary({ conditions }: { conditions: string }) {
  const { t } = useTranslation()
  try {
    const parsed: AutoGroupCondition[] = JSON.parse(conditions)
    if (!Array.isArray(parsed) || parsed.length === 0) {
      return <span className='text-muted-foreground'>-</span>
    }
    return (
      <div className='flex items-center gap-1'>
        <Badge variant='secondary'>{parsed.length} {t('conditions')}</Badge>
      </div>
    )
  } catch {
    return <span className='text-muted-foreground'>-</span>
  }
}

function EnabledSwitch({ rule }: { rule: AutoGroupRule }) {
  const { t } = useTranslation()
  const { triggerRefresh } = useAutoGroup()

  const handleToggle = async (checked: boolean) => {
    try {
      const conditions: AutoGroupCondition[] = JSON.parse(rule.conditions)
      const result = await updateRule({
        id: rule.id,
        name: rule.name,
        description: rule.description,
        enabled: checked,
        priority: rule.priority,
        target_group: rule.target_group,
        match_mode: rule.match_mode,
        conditions,
      })
      if (result.success) {
        toast.success(
          t(checked ? SUCCESS_MESSAGES.RULE_ENABLED : SUCCESS_MESSAGES.RULE_DISABLED)
        )
        triggerRefresh()
      }
    } catch {
      toast.error(t('Failed to update rule'))
    }
  }

  return (
    <Switch
      size='sm'
      checked={rule.enabled}
      onCheckedChange={handleToggle}
    />
  )
}

function RuleRowActions({ rule }: { rule: AutoGroupRule }) {
  const { t } = useTranslation()
  const { setOpen, setCurrentRule } = useAutoGroup()

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
            setCurrentRule(rule)
            setOpen('update-rule')
          }}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Edit size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            setCurrentRule(rule)
            setOpen('delete-rule')
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function useRulesColumns(): ColumnDef<AutoGroupRule>[] {
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
      accessorKey: 'name',
      meta: { label: t('Name'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[200px] truncate font-medium'>
          {row.getValue('name') || '-'}
        </div>
      ),
    },
    {
      accessorKey: 'priority',
      meta: { label: t('Priority'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Priority')} />
      ),
      cell: ({ row }) => <div>{row.getValue('priority')}</div>,
    },
    {
      accessorKey: 'target_group',
      meta: { label: t('Target Group') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Target Group')} />
      ),
      cell: ({ row }) => (
        <GroupBadge group={row.getValue('target_group')} />
      ),
    },
    {
      accessorKey: 'match_mode',
      meta: { label: t('Match Mode'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Match Mode')} />
      ),
      cell: ({ row }) => {
        const mode = row.getValue('match_mode') as string
        return (
          <Badge variant='secondary'>
            {mode === 'all' ? t('Match All') : t('Match Any')}
          </Badge>
        )
      },
    },
    {
      accessorKey: 'conditions',
      meta: { label: t('Conditions'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Conditions')} />
      ),
      cell: ({ row }) => {
        const conditions = row.getValue('conditions') as string
        return <ConditionsSummary conditions={conditions} />
      },
      enableSorting: false,
    },
    {
      accessorKey: 'enabled',
      meta: { label: t('Enabled'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Enabled')} />
      ),
      cell: ({ row }) => <EnabledSwitch rule={row.original} />,
      enableSorting: false,
    },
    {
      accessorKey: 'created_at',
      meta: { label: t('Created At'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created At')} />
      ),
      cell: ({ row }) => {
        const time = row.getValue('created_at') as number
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {time > 0 ? formatTimestampToDate(time) : '-'}
          </div>
        )
      },
    },
    {
      id: 'actions',
      cell: ({ row }) => <RuleRowActions rule={row.original} />,
    },
  ]
}
