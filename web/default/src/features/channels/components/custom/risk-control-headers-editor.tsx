import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { UseFormReturn } from 'react-hook-form'
import { Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ChannelFormValues } from '../../lib/channel-form'

interface HeaderRule {
  name: string
  source: string
  value: string
}

const ALLOWED_SOURCES = [
  'username',
  'user_id',
  'user_email',
  'user_group',
  'using_group',
  'token_id',
  'request_id',
  'custom',
] as const

function emptyRule(): HeaderRule {
  return { name: '', source: 'username', value: '' }
}

function normalizeRule(rule: HeaderRule): HeaderRule {
  const source = ALLOWED_SOURCES.includes(rule.source as (typeof ALLOWED_SOURCES)[number])
    ? rule.source
    : 'username'
  return {
    name: (rule.name ?? '').trim(),
    source,
    value: source === 'custom' ? rule.value ?? '' : '',
  }
}

function parse(val: string | undefined): HeaderRule[] {
  if (!val) return []
  try {
    const arr = JSON.parse(val)
    return Array.isArray(arr) ? arr.map(normalizeRule) : []
  } catch {
    return []
  }
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
}

export function RiskControlHeadersEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('risk_control_headers')
  const rules = useMemo(() => parse(raw), [raw])

  const setRules = (next: HeaderRule[]) => {
    const cleaned = next.map(normalizeRule).filter((r) => r.name)
    form.setValue(
      'risk_control_headers',
      cleaned.length > 0 ? JSON.stringify(cleaned) : '',
      { shouldDirty: true }
    )
  }

  const addRule = () => {
    form.setValue(
      'risk_control_headers',
      JSON.stringify([...rules, emptyRule()]),
      { shouldDirty: true }
    )
  }

  const removeRule = (index: number) => {
    const next = rules.filter((_, i) => i !== index)
    form.setValue(
      'risk_control_headers',
      next.length > 0 ? JSON.stringify(next) : '',
      { shouldDirty: true }
    )
  }

  const updateRule = (index: number, field: keyof HeaderRule, value: string) => {
    const next = rules.map((r, i) => (i === index ? { ...r, [field]: value } : r))
    form.setValue('risk_control_headers', JSON.stringify(next), {
      shouldDirty: true,
    })
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium">
          {t('Risk Control Headers')}
        </h4>
        <Button variant="outline" size="sm" onClick={addRule}>
          <Plus className="mr-1 h-3.5 w-3.5" />
          {t('Add Field')}
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {t('Configure HTTP headers sent to upstream for risk identification')}
      </p>

      {rules.map((rule, i) => (
        <div
          key={i}
          className="bg-muted/50 relative space-y-2 rounded-md border p-3"
        >
          <Button
            variant="ghost"
            size="icon"
            className="absolute right-2 top-2 h-6 w-6"
            onClick={() => removeRule(i)}
          >
            <X className="h-3.5 w-3.5" />
          </Button>

          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <div className="space-y-1">
              <Label className="text-xs">{t('Header Name')}</Label>
              <Input
                value={rule.name}
                onChange={(e) => updateRule(i, 'name', e.target.value)}
                onBlur={() => setRules(rules)}
                placeholder="X-User-Name"
                className="h-7 text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t('Source')}</Label>
              <Select
                value={rule.source}
                onValueChange={(v) => updateRule(i, 'source', v)}
              >
                <SelectTrigger className="h-7 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ALLOWED_SOURCES.map((src) => (
                    <SelectItem key={src} value={src}>
                      {t(`header_source_${src}`, src)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {rule.source === 'custom' && (
              <div className="space-y-1">
                <Label className="text-xs">{t('Custom Value')}</Label>
                <Input
                  value={rule.value}
                  onChange={(e) => updateRule(i, 'value', e.target.value)}
                  placeholder="{username} {user_id} ..."
                  className="h-7 text-xs"
                />
              </div>
            )}
          </div>
          {rule.source === 'custom' && (
            <p className="text-muted-foreground text-[10px]">
              {'{username} {user_id} {user_email} {user_group} {using_group} {token_id} {request_id}'}
            </p>
          )}
        </div>
      ))}

      {rules.length === 0 && (
        <p className="text-muted-foreground text-center text-xs py-2">
          {t('No fields configured')}
        </p>
      )}
      <Separator />
    </div>
  )
}
