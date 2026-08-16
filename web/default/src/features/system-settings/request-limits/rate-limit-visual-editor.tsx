/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { Plus, Search } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import {
  deleteGroupRateLimitRule,
  parseGroupRateLimitConfig,
  upsertGroupRateLimitRule,
  validateGroupRateLimitRule,
  type GroupRateLimitErrorCode,
  type GroupRateLimitRule,
} from './lib/group-rate-limit'
import { RateLimitDialog } from './rate-limit-dialog'

type RateLimitVisualEditorProps = {
  value: string
  onChange: (value: string) => void
}

const ERROR_MESSAGES: Record<GroupRateLimitErrorCode, string> = {
  'rule-invalid': 'Invalid group rate limit rule',
  'group-name-required': 'Group name is required',
  'group-name-control': 'Group name must not contain control characters',
  'group-name-duplicate': 'This group already has a rate limit',
  'total-count-range':
    'Total requests must be an integer between 0 and 2,147,483,647',
  'success-count-range':
    'Successful requests must be an integer between 1 and 2,147,483,647',
}

async function getGroups(): Promise<string[]> {
  const response = await api.get('/api/group/')
  const data = response.data?.data
  if (Array.isArray(data)) return data.map(String)
  if (data && typeof data === 'object') return Object.keys(data)
  return []
}

function getRuleErrors(
  rule: GroupRateLimitRule,
  rules: GroupRateLimitRule[],
  originalGroupName: string | null | undefined = undefined
): GroupRateLimitErrorCode[] {
  const errors = [...validateGroupRateLimitRule(rule).errors]
  let skippedOriginal = false
  const duplicate = rules.includes(rule)
    ? rules.filter((other) => other.groupName === rule.groupName).length > 1
    : rules.some((other) => {
        if (other.groupName !== rule.groupName) return false
        if (
          originalGroupName !== null &&
          originalGroupName !== undefined &&
          other.groupName === originalGroupName &&
          !skippedOriginal
        ) {
          skippedOriginal = true
          return false
        }
        return true
      })
  if (
    !errors.includes('group-name-required') &&
    duplicate
  ) {
    errors.push('group-name-duplicate')
  }
  return errors
}

function errorText(
  errors: GroupRateLimitErrorCode[],
  translate: (key: string) => string
) {
  return errors.map((error) => {
    return translate(ERROR_MESSAGES[error])
  })
}

export function RateLimitVisualEditor({
  value,
  onChange,
}: RateLimitVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<GroupRateLimitRule | null>(null)
  const groupsLoadedRef = useRef<string[] | null>(null)

  const { data: groupOptions = [] } = useQuery({
    queryKey: ['groups-list'],
    queryFn: async () => {
      if (groupsLoadedRef.current !== null) return groupsLoadedRef.current
      const groups = await getGroups()
      groupsLoadedRef.current = groups
      return groups
    },
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false,
  })

  const parsed = useMemo(() => parseGroupRateLimitConfig(value), [value])
  const rules = useMemo(() => (parsed.ok ? parsed.rules : []), [parsed])

  const filteredRules = useMemo(() => {
    const keyword = searchText.trim().toLowerCase()
    if (!keyword) return rules
    return rules.filter((rule) =>
      rule.groupName.toLowerCase().includes(keyword)
    )
  }, [rules, searchText])

  const documentGroupNames = useMemo(
    () => rules.map((rule) => rule.groupName),
    [rules]
  )

  function handleSave(data: GroupRateLimitRule): boolean {
    const errors = getRuleErrors(data, rules, editData?.groupName)
    if (errors.length > 0) {
      toast.error(errorText(errors, t).join('; '))
      return false
    }

    const result = upsertGroupRateLimitRule(
      value,
      data,
      editData?.groupName ?? null
    )
    if (!result.ok) {
      toast.error(t(result.error ?? 'Invalid group rate limit document'))
      return false
    }

    onChange(result.json)
    return true
  }

  function handleDelete(groupName: string) {
    const result = deleteGroupRateLimitRule(value, groupName)
    if (!result.ok) {
      toast.error(t(result.error ?? 'Invalid group rate limit document'))
      return
    }
    onChange(result.json)
  }

  if (!parsed.ok) {
    return (
      <div className='border-destructive/40 bg-destructive/10 text-destructive rounded-md border px-3 py-2 text-sm'>
        {t('Fix the JSON before switching to the visual editor.')}
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search group names...')}
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            className='pl-9'
          />
        </div>
        <Button
          onClick={() => {
            setEditData(null)
            setDialogOpen(true)
          }}
        >
          <Plus className='mr-2 h-4 w-4' />
          {t('Add group')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredRules}
        getRowKey={(rule) => rule.groupName}
        getRowClassName={(rule) =>
          getRuleErrors(rule, rules).length > 0
            ? 'bg-destructive/10'
            : undefined
        }
        emptyContent={
          searchText
            ? t('No groups match your search')
            : t(
                'No group-based rate limits configured. Click "Add group" to get started.'
              )
        }
        columns={[
          {
            id: 'group',
            header: t('Group Name'),
            cellClassName: (rule) =>
              getRuleErrors(rule, rules).length > 0
                ? 'text-destructive font-medium'
                : 'font-medium',
            cell: (rule) => (
              <div>
                <span>{rule.groupName}</span>
                {getRuleErrors(rule, rules).length > 0 ? (
                  <div className='text-destructive text-xs'>
                    {errorText(getRuleErrors(rule, rules), t).join('; ')}
                  </div>
                ) : null}
              </div>
            ),
          },
          {
            id: 'max-requests',
            header: t('Max Requests (incl. failures)'),
            className: 'text-right',
            cellClassName: (rule) =>
              getRuleErrors(rule, rules).length > 0
                ? 'text-destructive text-right'
                : 'text-right',
            cell: (rule) => (
              <span className='font-mono'>
                {rule.totalCount === 0
                  ? t('Unlimited')
                  : rule.totalCount.toLocaleString()}
              </span>
            ),
          },
          {
            id: 'max-success',
            header: t('Max Success'),
            className: 'text-right',
            cellClassName: (rule) =>
              getRuleErrors(rule, rules).length > 0
                ? 'text-destructive text-right'
                : 'text-right',
            cell: (rule) => (
              <span className='font-mono'>
                {rule.successCount.toLocaleString()}
              </span>
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (rule) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => {
                  setEditData(rule)
                  setDialogOpen(true)
                }}
                onDelete={() => handleDelete(rule.groupName)}
              />
            ),
          },
        ]}
      />

      <RateLimitDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        groupOptions={groupOptions}
        existingGroupNames={documentGroupNames}
      />
    </div>
  )
}
