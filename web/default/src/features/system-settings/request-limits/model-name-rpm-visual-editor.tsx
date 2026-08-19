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
import { useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

import {
  MODEL_NAME_RPM_MAX_GLOBAL,
  deleteModelNameRPMGroupTotalRule,
  deleteModelNameRPMRule,
  parseModelNameRPMConfig,
  upsertModelNameRPMGroupTotalRule,
  upsertModelNameRPMRule,
  validateModelNameRPMGroupTotalRule,
  type ModelNameRPMGroupTotalErrorCode,
  type ModelNameRPMGroupTotalRule,
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

const GROUP_TOTAL_ERROR_MESSAGES: Record<
  ModelNameRPMGroupTotalErrorCode,
  string
> = {
  'group-total-name-required': 'Group total name is required',
  'group-total-name-too-long':
    'Group total name must not exceed 64 characters',
  'group-total-name-whitespace':
    'Group total name must not contain whitespace or control characters',
  'group-total-name-duplicate': 'This group already has a total RPM limit',
  'group-total-rpm-range':
    'Total RPM must be an integer between 0 and 1,000,000 (0 means no total limit)',
  'group-total-user-rpm-range':
    'Per-user RPM must be an integer between 0 and 1,000,000 (0 means no per-user limit)',
  'group-total-user-rpm-exceeds-total':
    'Per-user RPM must not exceed the total RPM when the total limit is enabled',
  'group-total-without-limit':
    'Total RPM and per-user RPM cannot both be 0; delete the group entry instead',
}

const GROUP_TOTAL_FORM_ID = 'model-name-rpm-group-total-form'

export function ModelNameRPMVisualEditor({
  value,
  onChange,
}: ModelNameRPMVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<ModelNameRPMRule | null>(null)
  const [groupTotalDialogOpen, setGroupTotalDialogOpen] = useState(false)
  const [editingGroupName, setEditingGroupName] = useState<string | null>(null)
  const [groupTotalName, setGroupTotalName] = useState('')
  const [totalRpm, setTotalRpm] = useState(30)
  const [groupUserRpm, setGroupUserRpm] = useState(0)
  const [groupTotalError, setGroupTotalError] =
    useState<ModelNameRPMGroupTotalErrorCode | null>(null)

  const { data: groupOptions = [] } = useQuery({
    queryKey: ['groups-list'],
    queryFn: getGroups,
  })

  const { rules, groupTotals } = useMemo(() => {
    const parsed = parseModelNameRPMConfig(value)
    return parsed.ok
      ? { rules: parsed.rules, groupTotals: parsed.groupTotals }
      : { rules: [], groupTotals: [] }
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

  const openGroupTotalDialog = (rule?: ModelNameRPMGroupTotalRule) => {
    setEditingGroupName(rule?.groupName ?? null)
    setGroupTotalName(rule?.groupName ?? '')
    setTotalRpm(rule?.totalRpm ?? 30)
    setGroupUserRpm(rule?.userRpm ?? 0)
    setGroupTotalError(null)
    setGroupTotalDialogOpen(true)
  }

  const handleGroupTotalSave = (event: FormEvent) => {
    event.preventDefault()
    const rule = { groupName: groupTotalName, totalRpm, userRpm: groupUserRpm }
    const error = validateModelNameRPMGroupTotalRule(
      rule,
      groupTotals
        .map((item) => item.groupName)
        .filter((name) => name !== editingGroupName)
    )
    if (error) {
      setGroupTotalError(error.code)
      return
    }
    onChange(
      upsertModelNameRPMGroupTotalRule(value, editingGroupName, rule)
    )
    setGroupTotalDialogOpen(false)
  }

  const groupTotalErrorMessage = groupTotalError
    ? GROUP_TOTAL_ERROR_MESSAGES[groupTotalError]
    : null

  return (
    <div className='space-y-4'>
      <div className='space-y-3'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-1'>
            <h3 className='text-sm font-semibold'>{t('Group total RPM')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Top-level group limits apply across every model in the group, including models not listed in the models section: total_rpm caps all users combined, user_rpm caps each user, and 0 disables that limit.'
              )}
            </p>
          </div>
          <Button type='button' size='sm' onClick={() => openGroupTotalDialog()}>
            <Plus className='mr-2 h-4 w-4' />
            {t('Add group total')}
          </Button>
        </div>

        <StaticDataTable
          data={groupTotals}
          getRowKey={(rule) => rule.groupName}
          emptyContent={t('No group total RPM limits configured.')}
          columns={[
            {
              id: 'group-name',
              header: t('Group name'),
              cellClassName: 'font-medium',
              cell: (rule) => (
                <div className='flex flex-wrap items-center gap-2'>
                  <span>{rule.groupName}</span>
                  <Badge variant='secondary'>{t('All models combined')}</Badge>
                </div>
              ),
            },
            {
              id: 'total-rpm',
              header: t('Total RPM'),
              className: 'text-right',
              cellClassName: 'text-right font-mono',
              cell: (rule) =>
                rule.totalRpm === 0 ? (
                  <span className='text-muted-foreground text-xs'>
                    {t('None')}
                  </span>
                ) : (
                  <span className='font-mono'>
                    {rule.totalRpm.toLocaleString()}
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
              id: 'actions',
              header: t('Actions'),
              className: 'text-right',
              cellClassName: 'text-right',
              cell: (rule) => (
                <StaticRowActions
                  editLabel={t('Edit')}
                  deleteLabel={t('Delete')}
                  menuLabel={t('Open menu')}
                  onEdit={() => openGroupTotalDialog(rule)}
                  onDelete={() =>
                    onChange(
                      deleteModelNameRPMGroupTotalRule(value, rule.groupName)
                    )
                  }
                />
              ),
            },
          ]}
        />
      </div>

      <div className='border-border/70 flex items-center gap-4 border-t pt-4'>
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
          type='button'
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
            cell: (rule) =>
              rule.globalRpm === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('Unlimited')}
                </span>
              ) : (
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

      <Dialog
        open={groupTotalDialogOpen}
        onOpenChange={setGroupTotalDialogOpen}
        title={t('Group total RPM')}
        description={t(
          'Top-level group limits apply across every model in the group, including models not listed in the models section: total_rpm caps all users combined, user_rpm caps each user, and 0 disables that limit.'
        )}
        contentClassName='sm:max-w-md'
        contentHeight='auto'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setGroupTotalDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' form={GROUP_TOTAL_FORM_ID}>
              {editingGroupName === null ? t('Add') : t('Update')}
            </Button>
          </>
        }
      >
        <form
          id={GROUP_TOTAL_FORM_ID}
          className='space-y-4'
          onSubmit={handleGroupTotalSave}
        >
          <div className='space-y-2'>
            <Label htmlFor='model-name-rpm-group-total-name'>
              {t('Group name')}
            </Label>
            <Input
              id='model-name-rpm-group-total-name'
              value={groupTotalName}
              spellCheck={false}
              onChange={(event) => setGroupTotalName(event.target.value)}
            />
            {groupTotalError?.startsWith('group-total-name-') &&
              groupTotalErrorMessage && (
                <p className='text-destructive text-sm'>
                  {t(groupTotalErrorMessage)}
                </p>
              )}
          </div>
          <div className='space-y-2'>
            <Label htmlFor='model-name-rpm-group-total-rpm'>
              {t('Total RPM')}
            </Label>
            <Input
              id='model-name-rpm-group-total-rpm'
              type='number'
              min={0}
              max={MODEL_NAME_RPM_MAX_GLOBAL}
              step={1}
              value={totalRpm}
              onChange={(event) =>
                setTotalRpm(
                  event.target.value === '' ? 0 : event.target.valueAsNumber
                )
              }
            />
            {(groupTotalError === 'group-total-rpm-range' ||
              groupTotalError === 'group-total-without-limit') &&
              groupTotalErrorMessage && (
                <p className='text-destructive text-sm'>
                  {t(groupTotalErrorMessage)}
                </p>
              )}
          </div>
          <div className='space-y-2'>
            <Label htmlFor='model-name-rpm-group-user-rpm'>
              {t('Per-user RPM')}
            </Label>
            <Input
              id='model-name-rpm-group-user-rpm'
              type='number'
              min={0}
              max={MODEL_NAME_RPM_MAX_GLOBAL}
              step={1}
              value={groupUserRpm}
              onChange={(event) =>
                setGroupUserRpm(
                  event.target.value === '' ? 0 : event.target.valueAsNumber
                )
              }
            />
            {(groupTotalError === 'group-total-user-rpm-range' ||
              groupTotalError === 'group-total-user-rpm-exceeds-total') &&
              groupTotalErrorMessage && (
                <p className='text-destructive text-sm'>
                  {t(groupTotalErrorMessage)}
                </p>
              )}
          </div>
        </form>
      </Dialog>
    </div>
  )
}
