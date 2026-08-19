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
import { Plus, X } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  MODEL_NAME_RPM_MAX_GLOBAL,
  validateModelNameRPMRule,
  type ModelNameRPMGroupLimit,
  type ModelNameRPMRule,
  type ModelNameRPMRuleErrorCode,
} from './lib/model-name-rpm'

const ERROR_MESSAGES: Record<ModelNameRPMRuleErrorCode, string> = {
  'model-name-required': 'Model name is required',
  'model-name-too-long': 'Model name must not exceed 255 characters',
  'model-name-whitespace':
    'Model name must not contain whitespace or control characters',
  'model-name-duplicate': 'This model already has a rule',
  'global-rpm-range':
    'Global RPM must be an integer between 0 and 1,000,000 (0 means unlimited)',
  'unlimited-without-sublimit':
    'When the global RPM is 0 (unlimited), configure at least one per-user or per-group limit; otherwise delete this model rule',
  'user-rpm-range': 'Per-user RPM must be a positive integer when set',
  'user-rpm-exceeds-global':
    'Per-user RPM must not exceed the global RPM',
  'group-name-required': 'Select a group',
  'group-name-too-long': 'Group name must not exceed 64 characters',
  'group-name-whitespace':
    'Group name must not contain whitespace or control characters',
  'group-name-duplicate': 'This group already has a limit for this model',
  'group-rpm-range': 'Group RPM must be an integer greater than 0',
  'group-rpm-exceeds-global': 'Group RPM must not exceed the global RPM',
}

const MODEL_NAME_RPM_DIALOG_FORM_ID = 'model-name-rpm-form'

type ModelNameRPMDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (previousModelName: string | null, rule: ModelNameRPMRule) => void
  editData?: ModelNameRPMRule | null
  existingModelNames: string[]
  groupOptions: string[]
}

type EditableGroupLimit = ModelNameRPMGroupLimit & {
  rowKey: number
}

export function ModelNameRPMDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  existingModelNames,
  groupOptions,
}: ModelNameRPMDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData

  const [modelName, setModelName] = useState('')
  const [globalRpm, setGlobalRpm] = useState(60)
  const [userRpm, setUserRpm] = useState(0)
  const [groups, setGroups] = useState<EditableGroupLimit[]>([])
  const [errorCode, setErrorCode] =
    useState<ModelNameRPMRuleErrorCode | null>(null)
  const [errorGroupIndex, setErrorGroupIndex] = useState<number | null>(null)
  const nextGroupRowKey = useRef(0)

  useEffect(() => {
    setErrorCode(null)
    setErrorGroupIndex(null)
    setModelName(editData?.modelName ?? '')
    setGlobalRpm(editData?.globalRpm ?? 60)
    setUserRpm(editData?.userRpm ?? 0)
    nextGroupRowKey.current = 0
    setGroups(
      editData
        ? editData.groups.map((group) => ({
            ...group,
            rowKey: nextGroupRowKey.current++,
          }))
        : []
    )
  }, [editData, open])

  // A group already present in the document must stay selectable even if it was
  // removed from the group catalog, otherwise editing would silently drop it.
  const selectableGroups = [
    ...new Set([...groupOptions, ...groups.map((group) => group.groupName)]),
  ].filter((group) => group !== '')

  const updateGroup = (index: number, patch: Partial<ModelNameRPMGroupLimit>) =>
    setGroups((previous) =>
      previous.map((group, current) =>
        current === index ? { ...group, ...patch } : group
      )
    )

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault()
    const rule: ModelNameRPMRule = {
      modelName,
      globalRpm,
      userRpm,
      groups: groups.map((group) => ({
        groupName: group.groupName,
        rpm: group.rpm,
      })),
    }
    const error = validateModelNameRPMRule(rule, existingModelNames)
    if (error) {
      setErrorCode(error.code)
      setErrorGroupIndex(error.groupIndex ?? null)
      return
    }
    onSave(editData?.modelName ?? null, rule)
    onOpenChange(false)
  }

  const fieldError = (
    code: ModelNameRPMRuleErrorCode | ModelNameRPMRuleErrorCode[],
    groupIndex?: number
  ) => {
    if (!errorCode) return null
    const codes = Array.isArray(code) ? code : [code]
    if (!codes.includes(errorCode)) return null
    if (groupIndex !== undefined && errorGroupIndex !== groupIndex) return null
    if (groupIndex === undefined && errorGroupIndex !== null) return null
    return (
      <p className='text-destructive text-sm'>
        {t(ERROR_MESSAGES[errorCode])}
      </p>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEditMode ? t('Edit model RPM rule') : t('Add model RPM rule')}
      description={t(
        'Requests consume the global bucket and, when configured, the matching group and per-user buckets.'
      )}
      contentClassName='sm:max-w-[560px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={MODEL_NAME_RPM_DIALOG_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <form
        id={MODEL_NAME_RPM_DIALOG_FORM_ID}
        onSubmit={handleSubmit}
        className='space-y-4'
      >
        <div className='space-y-2'>
          <Label htmlFor='model-name-rpm-model'>{t('Model name')}</Label>
          <Input
            id='model-name-rpm-model'
            value={modelName}
            spellCheck={false}
            placeholder={t('e.g., gpt-4o')}
            onChange={(event) => setModelName(event.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Must match the model name the client requests, including aliases.'
            )}
          </p>
          {fieldError([
            'model-name-required',
            'model-name-too-long',
            'model-name-whitespace',
            'model-name-duplicate',
          ])}
        </div>

        <div className='space-y-2'>
          <Label htmlFor='model-name-rpm-global'>{t('Global RPM')}</Label>
          <Input
            id='model-name-rpm-global'
            type='number'
            min={0}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            value={globalRpm}
            onChange={(event) =>
              setGlobalRpm(Number.parseInt(event.target.value, 10) || 0)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t('Hard ceiling shared by every group, in requests per minute.')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('0 means unlimited; usage is still counted and displayed.')}
          </p>
          {fieldError(['global-rpm-range', 'unlimited-without-sublimit'])}
        </div>

        <div className='space-y-2'>
          <Label htmlFor='model-name-rpm-user'>{t('Per-user RPM')}</Label>
          <Input
            id='model-name-rpm-user'
            type='number'
            min={1}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            value={userRpm || ''}
            onChange={(event) =>
              setUserRpm(
                event.target.value === '' ? 0 : event.target.valueAsNumber
              )
            }
          />
          {fieldError(['user-rpm-range', 'user-rpm-exceeds-global'])}
        </div>

        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <Label>{t('Group sub-limits')}</Label>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setGroups((previous) => [
                  ...previous,
                  {
                    groupName: '',
                    rpm: 1,
                    rowKey: nextGroupRowKey.current++,
                  },
                ])
              }
            >
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group limit')}
            </Button>
          </div>

          {groups.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('No group sub-limit. Only the global RPM applies.')}
            </p>
          ) : (
            groups.map((group, index) => (
              <div key={group.rowKey} className='space-y-1'>
                <div className='flex items-center gap-2'>
                  <Select
                    value={group.groupName}
                    onValueChange={(value: string) =>
                      updateGroup(index, { groupName: value })
                    }
                  >
                    <SelectTrigger className='flex-1'>
                      <SelectValue placeholder={t('Select a group')} />
                    </SelectTrigger>
                    <SelectContent>
                      {selectableGroups.map((option) => (
                        <SelectItem key={option} value={option}>
                          {option}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Input
                    type='number'
                    min={1}
                    max={MODEL_NAME_RPM_MAX_GLOBAL}
                    step={1}
                    className='w-32'
                    aria-label={t('Group RPM')}
                    value={group.rpm}
                    onChange={(event) =>
                      updateGroup(index, {
                        rpm: Number.parseInt(event.target.value, 10) || 0,
                      })
                    }
                  />
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    aria-label={t('Remove group limit')}
                    onClick={() =>
                      setGroups((previous) =>
                        previous.filter((_, current) => current !== index)
                      )
                    }
                  >
                    <X className='h-4 w-4' />
                  </Button>
                </div>
                {fieldError(
                  [
                    'group-name-required',
                    'group-name-too-long',
                    'group-name-whitespace',
                    'group-name-duplicate',
                    'group-rpm-range',
                    'group-rpm-exceeds-global',
                  ],
                  index
                )}
              </div>
            ))
          )}
        </div>
      </form>
    </Dialog>
  )
}
