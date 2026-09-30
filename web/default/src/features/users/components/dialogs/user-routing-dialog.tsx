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
import { ArrowDown, ArrowUp, Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import {
  getGroups,
  getUserRouting,
  saveUserRouting,
  type UserRoutingRule,
} from '../../api'
import type { User } from '../../types'

type EditableRule = UserRoutingRule & { key: string }

export function UserRoutingDialog({
  user,
  onOpenChange,
}: {
  user: User
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [rules, setRules] = useState<EditableRule[]>([])
  const [groups, setGroups] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [failed, setFailed] = useState(false)
  const [reload, setReload] = useState(0)

  useEffect(() => {
    let active = true
    setLoading(true)
    setFailed(false)
    void Promise.all([getUserRouting(user.id), getGroups()])
      .then(([policy, available]) => {
        if (!active) return
        if (!policy.success || !available.success) {
          throw new Error(policy.message || available.message)
        }
        setRules(
          (policy.data ?? []).map((rule) => ({
            ...rule,
            key: crypto.randomUUID(),
          }))
        )
        setGroups((available.data ?? []).filter((group) => group !== 'auto'))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
      .catch(() => {
        if (!active) return
        setFailed(true)
        toast.error(t('Operation failed'))
      })
    return () => {
      active = false
    }
  }, [user.id, reload, t])

  const update = (key: string, patch: Partial<UserRoutingRule>) => {
    setRules((current) =>
      current.map((rule) => (rule.key === key ? { ...rule, ...patch } : rule))
    )
  }
  const move = (index: number, offset: number) => {
    setRules((current) => {
      const next = [...current]
      ;[next[index], next[index + offset]] = [next[index + offset], next[index]]
      return next
    })
  }
  const save = async () => {
    setSaving(true)
    try {
      const result = await saveUserRouting(
        user.id,
        rules.map(({ source_group, model, target_group, enabled }) => ({
          source_group,
          model,
          target_group,
          enabled,
        }))
      )
      if (!result.success) {
        toast.error(result.message || t('Operation failed'))
        return
      }
      toast.success(t('Saved successfully'))
      onOpenChange(false)
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!saving) onOpenChange(open)
      }}
      title={`${t('User Routing')} · ${user.username}`}
      contentClassName='sm:max-w-3xl'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={save}
            disabled={
              loading ||
              failed ||
              saving ||
              rules.some((rule) => !groups.includes(rule.target_group))
            }
          >
            <Save className='size-4' />
            {t('Save')}
          </Button>
        </>
      }
    >
      {loading && <p role='status'>{t('Loading...')}</p>}
      {!loading && failed && (
        <Button
          variant='outline'
          onClick={() => setReload((value) => value + 1)}
        >
          {t('Retry')}
        </Button>
      )}
      {!loading && !failed && (
        <div className='space-y-3'>
          {rules.length === 0 && (
            <p className='text-muted-foreground py-6 text-center text-sm'>
              {t('No routing rules')}
            </p>
          )}
          {rules.map((rule, index) => (
            <div key={rule.key} className='rounded-lg border p-3'>
              <div className='mb-3 flex items-center justify-between gap-2'>
                <span className='text-muted-foreground text-sm'>
                  {t('Priority')} {index + 1}
                </span>
                <div className='flex items-center gap-2'>
                  <Switch
                    aria-label={t('Enabled')}
                    checked={rule.enabled}
                    disabled={saving}
                    onCheckedChange={(enabled) => update(rule.key, { enabled })}
                  />
                  {[
                    {
                      Icon: ArrowUp,
                      label: t('Move up'),
                      disabled: index === 0,
                      action: () => move(index, -1),
                    },
                    {
                      Icon: ArrowDown,
                      label: t('Move down'),
                      disabled: index === rules.length - 1,
                      action: () => move(index, 1),
                    },
                    {
                      Icon: Trash2,
                      label: t('Delete'),
                      disabled: false,
                      action: () =>
                        setRules((current) =>
                          current.filter((item) => item.key !== rule.key)
                        ),
                    },
                  ].map(({ Icon, label, disabled, action }) => (
                    <Tooltip key={label}>
                      <TooltipTrigger
                        render={
                          <Button
                            size='icon'
                            variant='ghost'
                            aria-label={label}
                            disabled={saving || disabled}
                            onClick={action}
                          >
                            <Icon className='size-4' />
                          </Button>
                        }
                      />
                      <TooltipContent>{label}</TooltipContent>
                    </Tooltip>
                  ))}
                </div>
              </div>
              <FieldGroup className='grid gap-3 sm:grid-cols-3'>
                <Field>
                  <FieldLabel htmlFor={`${rule.key}-source`}>
                    {t('Original group')}
                  </FieldLabel>
                  <NativeSelect
                    id={`${rule.key}-source`}
                    className='w-full'
                    value={rule.source_group}
                    disabled={saving}
                    onChange={(e) =>
                      update(rule.key, { source_group: e.target.value })
                    }
                  >
                    <NativeSelectOption value=''>
                      {t('All groups')}
                    </NativeSelectOption>
                    {[
                      ...new Set(
                        [...groups, rule.source_group].filter(Boolean)
                      ),
                    ].map((group) => (
                      <NativeSelectOption key={group} value={group}>
                        {group}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.key}-model`}>
                    {t('Model')}
                  </FieldLabel>
                  <Input
                    id={`${rule.key}-model`}
                    value={rule.model}
                    maxLength={200}
                    disabled={saving}
                    placeholder={t('All models')}
                    onChange={(e) =>
                      update(rule.key, { model: e.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.key}-target`}>
                    {t('Target group')}
                  </FieldLabel>
                  <NativeSelect
                    id={`${rule.key}-target`}
                    className='w-full'
                    value={rule.target_group}
                    disabled={saving}
                    onChange={(e) =>
                      update(rule.key, { target_group: e.target.value })
                    }
                  >
                    <NativeSelectOption value=''>
                      {t('Select group')}
                    </NativeSelectOption>
                    {[
                      ...new Set(
                        [...groups, rule.target_group].filter(Boolean)
                      ),
                    ].map((group) => (
                      <NativeSelectOption key={group} value={group}>
                        {group}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
              </FieldGroup>
            </div>
          ))}
          <Button
            variant='outline'
            disabled={saving || rules.length >= 32}
            onClick={() =>
              setRules((current) => [
                ...current,
                {
                  key: crypto.randomUUID(),
                  source_group: '',
                  model: '',
                  target_group: '',
                  enabled: true,
                },
              ])
            }
          >
            <Plus className='size-4' />
            {t('Add routing rule')}
          </Button>
        </div>
      )}
    </Dialog>
  )
}
