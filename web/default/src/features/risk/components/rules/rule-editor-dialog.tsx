import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, X } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { RiskRule, RiskCondition } from '../../api'
import { createRiskRule, updateRiskRule } from '../../api'
import {
  emptyRuleForm,
  safeParseJSON,
  sanitizeConditionsForScope,
  getMetricOptionsForScope,
  getDefaultMetricForScope,
  OP_OPTIONS,
  SCOPE_OPTIONS,
  ACTION_OPTIONS,
  MATCH_MODE_OPTIONS,
  METRIC_LABEL_MAP,
} from '../../constants'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialRule: RiskRule | null
  groupOptions: string[]
  enabledGroupSet: Set<string>
  onSaved: () => void
}

export function RuleEditorDialog({
  open,
  onOpenChange,
  initialRule,
  groupOptions,
  enabledGroupSet,
  onSaved,
}: Props) {
  const { t } = useTranslation()
  const [form, setForm] = useState(emptyRuleForm())
  const [notice, setNotice] = useState('')

  useEffect(() => {
    if (!open) return
    if (initialRule) {
      const next = {
        ...emptyRuleForm(),
        ...initialRule,
        conditions: safeParseJSON<RiskCondition[]>(initialRule.conditions, [
          { metric: 'distinct_ip_10m', op: '>=', value: 3 },
        ]),
        groups: safeParseJSON<string[]>(initialRule.groups, []),
      }
      const san = sanitizeConditionsForScope(next.conditions, next.scope)
      setForm({ ...next, conditions: san.conditions })
      setNotice(
        san.changed
          ? t('Some metrics were replaced for this scope')
          : ''
      )
    } else {
      setForm(emptyRuleForm())
      setNotice('')
    }
  }, [open, initialRule, t])

  const saveMutation = useMutation({
    mutationFn: (data: ReturnType<typeof emptyRuleForm>) => {
      return data.id
        ? updateRiskRule(data.id, data)
        : createRiskRule(data)
    },
    onSuccess: () => {
      toast.success(t('Rule saved'))
      onSaved()
    },
  })

  const updateField = (field: string, value: unknown) => {
    if (field === 'scope') {
      const san = sanitizeConditionsForScope(
        form.conditions,
        value as string
      )
      setForm((p) => ({
        ...p,
        scope: value as string,
        conditions: san.conditions,
      }))
      setNotice(
        san.changed ? t('Some metrics were replaced for this scope') : ''
      )
      return
    }
    setForm((p) => ({ ...p, [field]: value }))
  }

  const updateCondition = (
    index: number,
    field: string,
    value: unknown
  ) => {
    setForm((p) => ({
      ...p,
      conditions: p.conditions.map((c, i) =>
        i === index ? { ...c, [field]: value } : c
      ),
    }))
  }

  const addCondition = () => {
    setForm((p) => ({
      ...p,
      conditions: [
        ...p.conditions,
        { metric: getDefaultMetricForScope(p.scope), op: '>=', value: 1 },
      ],
    }))
  }

  const removeCondition = (index: number) => {
    setForm((p) => ({
      ...p,
      conditions: p.conditions.filter((_, i) => i !== index),
    }))
  }

  const toggleGroup = (name: string) => {
    setForm((p) => ({
      ...p,
      groups: p.groups.includes(name)
        ? p.groups.filter((g) => g !== name)
        : [...p.groups, name],
    }))
  }

  const availableMetrics = useMemo(
    () => getMetricOptionsForScope(form.scope),
    [form.scope]
  )

  const handleSubmit = () => {
    if (!form.name.trim()) {
      toast.error(t('Rule name is required'))
      return
    }
    if (!form.conditions.length) {
      toast.error(t('At least one condition is required'))
      return
    }
    if (form.enabled && form.groups.length === 0) {
      toast.error(
        t('Rule must be bound to at least one group before enabling')
      )
      return
    }
    saveMutation.mutate(form)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {form.id ? t('Edit Rule') : t('Create Rule')}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {notice && (
            <Alert>
              <AlertDescription>{notice}</AlertDescription>
            </Alert>
          )}

          {/* Basic Info */}
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label>{t('Rule Name')}</Label>
              <Input
                value={form.name}
                onChange={(e) => updateField('name', e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Scope')}</Label>
              <Select
                value={form.scope}
                onValueChange={(v) => updateField('scope', v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SCOPE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>{t('Match Mode')}</Label>
              <Select
                value={form.match_mode}
                onValueChange={(v) => updateField('match_mode', v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MATCH_MODE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {t(o.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>{t('Action')}</Label>
              <Select
                value={form.action}
                onValueChange={(v) => updateField('action', v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ACTION_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {t(o.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>{t('Priority')}</Label>
              <Input
                type="number"
                value={form.priority}
                onChange={(e) =>
                  updateField('priority', Number(e.target.value))
                }
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Score Weight')}</Label>
              <Input
                type="number"
                value={form.score_weight}
                onChange={(e) =>
                  updateField('score_weight', Number(e.target.value))
                }
              />
            </div>
          </div>

          {/* Conditions */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>{t('Conditions')}</Label>
              <Button
                variant="outline"
                size="sm"
                onClick={addCondition}
              >
                <Plus className="mr-1 h-3.5 w-3.5" />
                {t('Add Condition')}
              </Button>
            </div>
            {form.conditions.map((c, i) => (
              <div key={i} className="flex items-center gap-2">
                <Select
                  value={c.metric}
                  onValueChange={(v) => updateCondition(i, 'metric', v)}
                >
                  <SelectTrigger className="w-[180px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {availableMetrics.map((m) => (
                      <SelectItem key={m.value} value={m.value}>
                        {t(m.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={c.op}
                  onValueChange={(v) => updateCondition(i, 'op', v)}
                >
                  <SelectTrigger className="w-[80px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {OP_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  type="number"
                  value={c.value}
                  onChange={(e) =>
                    updateCondition(i, 'value', Number(e.target.value))
                  }
                  className="w-[100px]"
                />
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0"
                  onClick={() => removeCondition(i)}
                >
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>

          {/* Groups */}
          <div className="space-y-2">
            <Label>{t('Groups')}</Label>
            <div className="flex flex-wrap gap-2">
              {groupOptions.map((g) => (
                <Button
                  key={g}
                  variant={form.groups.includes(g) ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => toggleGroup(g)}
                >
                  {g}
                </Button>
              ))}
              {groupOptions.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  {t('No groups available')}
                </p>
              )}
            </div>
          </div>

          {/* Recovery */}
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="flex items-center gap-3">
              <Label>{t('Enabled')}</Label>
              <Switch
                checked={form.enabled}
                onCheckedChange={(v) => updateField('enabled', v)}
              />
            </div>
            <div className="flex items-center gap-3">
              <Label>{t('Auto Block')}</Label>
              <Switch
                checked={form.auto_block}
                onCheckedChange={(v) => updateField('auto_block', v)}
              />
            </div>
            <div className="flex items-center gap-3">
              <Label>{t('Auto Recover')}</Label>
              <Switch
                checked={form.auto_recover}
                onCheckedChange={(v) => updateField('auto_recover', v)}
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Recovery Seconds')}</Label>
              <Input
                type="number"
                value={form.recover_after_seconds}
                onChange={(e) =>
                  updateField(
                    'recover_after_seconds',
                    Number(e.target.value)
                  )
                }
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Response Status Code')}</Label>
              <Input
                type="number"
                value={form.response_status_code}
                onChange={(e) =>
                  updateField(
                    'response_status_code',
                    Number(e.target.value)
                  )
                }
              />
            </div>
            <div className="col-span-full space-y-1">
              <Label>{t('Response Message')}</Label>
              <Textarea
                value={form.response_message}
                onChange={(e) =>
                  updateField('response_message', e.target.value)
                }
                rows={2}
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label>{t('Description')}</Label>
            <Textarea
              value={form.description}
              onChange={(e) =>
                updateField('description', e.target.value)
              }
              rows={2}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={saveMutation.isPending}
          >
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
