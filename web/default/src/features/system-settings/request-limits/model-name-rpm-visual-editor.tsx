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
import { useQuery } from '@tanstack/react-query'
import { Plus, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import {
  deleteModelNameRPMRule,
  parseModelNameRPMConfig,
  upsertModelNameRPMRule,
  type ModelNameRPMRule,
} from './lib/model-name-rpm'
import { ModelNameRPMDialog } from './model-name-rpm-dialog'

// /api/group/ serves an in-memory ratio snapshot and never touches the
// database, so the dropdown costs nothing beyond one cached request.
async function getGroups(): Promise<string[]> {
  const res = await api.get('/api/group/')
  const data = res.data?.data
  if (Array.isArray(data)) return data.map(String)
  if (data && typeof data === 'object') return Object.keys(data)
  return []
}

type ModelNameRPMVisualEditorProps = {
  value: string
  onChange: (value: string) => void
}

export function ModelNameRPMVisualEditor({
  value,
  onChange,
}: ModelNameRPMVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<ModelNameRPMRule | null>(null)

  const { data: groupOptions = [] } = useQuery({
    queryKey: ['groups-list'],
    queryFn: getGroups,
  })

  const rules = useMemo(() => {
    const parsed = parseModelNameRPMConfig(value)
    return parsed.ok ? parsed.rules : []
  }, [value])

  const filteredRules = useMemo(() => {
    if (!searchText) return rules
    const lowerSearch = searchText.toLowerCase()
    return rules.filter((rule) =>
      rule.modelName.toLowerCase().includes(lowerSearch)
    )
  }, [rules, searchText])

  const handleSave = (
    previousModelName: string | null,
    rule: ModelNameRPMRule
  ) => onChange(upsertModelNameRPMRule(value, previousModelName, rule))

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search model names...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
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
          {t('Add model')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredRules}
        getRowKey={(rule) => rule.modelName}
        emptyContent={
          searchText
            ? t('No models match your search')
            : t(
                'No model RPM rules configured. Click "Add model" to get started.'
              )
        }
        columns={[
          {
            id: 'model',
            header: t('Model name'),
            cellClassName: 'font-medium',
            cell: (rule) => rule.modelName,
          },
          {
            id: 'global-rpm',
            header: t('Global RPM'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (rule) => (
              <span className='font-mono'>
                {rule.globalRpm.toLocaleString()}
              </span>
            ),
          },
          {
            id: 'user-rpm',
            header: t('Per-user RPM'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (rule) =>
              rule.userRpm === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('None')}
                </span>
              ) : (
                <span className='font-mono'>
                  {rule.userRpm.toLocaleString()}
                </span>
              ),
          },
          {
            id: 'group-rpm',
            header: t('Group sub-limits'),
            cell: (rule) =>
              rule.groups.length === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('None')}
                </span>
              ) : (
                <div className='flex flex-wrap gap-1'>
                  {rule.groups.map((group) => (
                    <Badge key={group.groupName} variant='secondary'>
                      {group.groupName}: {group.rpm.toLocaleString()}
                    </Badge>
                  ))}
                </div>
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
                onDelete={() =>
                  onChange(deleteModelNameRPMRule(value, rule.modelName))
                }
              />
            ),
          },
        ]}
      />

      <ModelNameRPMDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        existingModelNames={rules
          .map((rule) => rule.modelName)
          .filter((name) => name !== editData?.modelName)}
        groupOptions={groupOptions}
      />
    </div>
  )
}
