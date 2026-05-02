import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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
import type { ModerationRule, ModerationCategory, ModerationRuleCondition } from '../../api'
import { createModerationRule, updateModerationRule } from '../../api'
import {
  emptyModerationRuleForm,
  safeParseJSON,
  OP_OPTIONS,
  MODERATION_ACTION_OPTIONS,
} from '../../constants'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialRule: ModerationRule | null
  categories: ModerationCategory[]
  onSaved: () => void
}

export function ModerationRuleEditorDialog({
  open,
  onOpenChange,
  initialRule,
  categories,
  onSaved,
}: Props) {
  const { t } = useTranslation()
  const [form, setForm] = useState(emptyModerationRuleForm())

  useEffect(() => {
    if (!open) return
    if (initialRule) {
      setForm({
        ...emptyModerationRuleForm(),
        ...initialRule,
        conditions: safeParseJSON<ModerationRuleCondition[]>(
          initialRule.conditions,
          [{ category: '', op: '>=', threshold: 0.5 }]
        ),
      })
    } else {
      setForm(emptyModerationRuleForm())
    }
  }, [open, initialRule])

  const saveMutation = useMutation({
    mutationFn: (data: typeof form) => {
      return data.id
        ? updateModerationRule(data.id, data as Partial<ModerationRule>)
        : createModerationRule(data as Partial<ModerationRule>)
    },
    onSuccess: () => {
      toast.success(t('Rule saved'))
      onSaved()
    },
  })

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
        { category: categories[0]?.key || '', op: '>=', threshold: 0.5 },
      ],
    }))
  }

  const removeCondition = (index: number) => {
    setForm((p) => ({
      ...p,
      conditions: p.conditions.filter((_, i) => i !== index),
    }))
  }

  const handleSubmit = () => {
    if (!form.name.trim()) {
      toast.error(t('Rule name is required'))
      return
    }
    if (!form.conditions.length) {
      toast.error(t('At least one condition is required'))
      return
    }
    saveMutation.mutate(form)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {form.id ? t('Edit Moderation Rule') : t('Create Moderation Rule')}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>{t('Rule Name')}</Label>
              <Input
                value={form.name}
                onChange={(e) =>
                  setForm((p) => ({ ...p, name: e.target.value }))
                }
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Logic')}</Label>
              <Select
                value={form.logic}
                onValueChange={(v) =>
                  setForm((p) => ({ ...p, logic: v }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="or">OR</SelectItem>
                  <SelectItem value="and">AND</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>{t('Action')}</Label>
              <Select
                value={form.action}
                onValueChange={(v) =>
                  setForm((p) => ({ ...p, action: v }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODERATION_ACTION_OPTIONS.map((o) => (
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
                  setForm((p) => ({
                    ...p,
                    priority: Number(e.target.value),
                  }))
                }
              />
            </div>
          </div>

          <div className="flex items-center gap-3">
            <Label>{t('Enabled')}</Label>
            <Switch
              checked={form.enabled}
              onCheckedChange={(v) =>
                setForm((p) => ({ ...p, enabled: v }))
              }
            />
          </div>

          {/* Conditions */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>{t('Conditions')}</Label>
              <Button variant="outline" size="sm" onClick={addCondition}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                {t('Add Condition')}
              </Button>
            </div>
            {form.conditions.map((c, i) => (
              <div key={i} className="flex items-center gap-2">
                <Select
                  value={c.category}
                  onValueChange={(v) =>
                    updateCondition(i, 'category', v)
                  }
                >
                  <SelectTrigger className="w-[200px]">
                    <SelectValue placeholder={t('Category')} />
                  </SelectTrigger>
                  <SelectContent>
                    {categories.map((cat) => (
                      <SelectItem key={cat.key} value={cat.key}>
                        {cat.label}
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
                  step="0.01"
                  min="0"
                  max="1"
                  value={c.threshold}
                  onChange={(e) =>
                    updateCondition(i, 'threshold', Number(e.target.value))
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

          <div className="space-y-1">
            <Label>{t('Description')}</Label>
            <Input
              value={form.description}
              onChange={(e) =>
                setForm((p) => ({ ...p, description: e.target.value }))
              }
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
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
