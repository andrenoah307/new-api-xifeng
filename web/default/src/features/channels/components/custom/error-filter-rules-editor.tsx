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

interface FilterRule {
  status_codes: number[]
  message_contains: string[]
  error_codes: string[]
  action: string
  rewrite_message: string
  replace_status_code: number
  replace_message: string
}

function emptyRule(): FilterRule {
  return {
    status_codes: [],
    message_contains: [],
    error_codes: [],
    action: 'retry',
    rewrite_message: '',
    replace_status_code: 200,
    replace_message: '',
  }
}

function parse(val: string | undefined): FilterRule[] {
  if (!val) return []
  try {
    return JSON.parse(val)
  } catch {
    return []
  }
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
  channelId?: number
}

export function ErrorFilterRulesEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('error_filter_rules')
  const rules = useMemo(() => parse(raw), [raw])

  const setRules = (next: FilterRule[]) => {
    form.setValue('error_filter_rules', JSON.stringify(next), {
      shouldDirty: true,
    })
  }

  const addRule = () => setRules([...rules, emptyRule()])

  const removeRule = (index: number) =>
    setRules(rules.filter((_, i) => i !== index))

  const updateRule = (
    index: number,
    field: keyof FilterRule,
    value: unknown
  ) => {
    setRules(rules.map((r, i) => (i === index ? { ...r, [field]: value } : r)))
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium">{t('Error Filter Rules')}</h4>
        <Button type="button" variant="outline" size="sm" onClick={addRule}>
          <Plus className="mr-1 h-3.5 w-3.5" />
          {t('Add Rule')}
        </Button>
      </div>
      {rules.map((rule, i) => (
        <div
          key={i}
          className="bg-muted/50 relative space-y-2 rounded-md border p-3"
        >
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-2 top-2 h-6 w-6"
            onClick={() => removeRule(i)}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <div className="space-y-1">
              <Label className="text-xs">{t('Status Codes')}</Label>
              <Input
                value={rule.status_codes.join(', ')}
                onChange={(e) =>
                  updateRule(
                    i,
                    'status_codes',
                    e.target.value
                      .split(',')
                      .map((s) => Number(s.trim()))
                      .filter(Boolean)
                  )
                }
                placeholder="400, 500"
                className="h-7 text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t('Message Keywords')}</Label>
              <Input
                value={rule.message_contains.join(', ')}
                onChange={(e) =>
                  updateRule(
                    i,
                    'message_contains',
                    e.target.value
                      .split(',')
                      .map((s) => s.trim())
                      .filter(Boolean)
                  )
                }
                className="h-7 text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t('Action')}</Label>
              <Select
                value={rule.action}
                onValueChange={(v) => updateRule(i, 'action', v)}
              >
                <SelectTrigger className="h-7 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="retry">{t('Retry')}</SelectItem>
                  <SelectItem value="rewrite">{t('Rewrite')}</SelectItem>
                  <SelectItem value="replace">{t('Replace')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          {rule.action === 'rewrite' && (
            <div className="space-y-1">
              <Label className="text-xs">{t('Rewrite Message')}</Label>
              <Input
                value={rule.rewrite_message}
                onChange={(e) =>
                  updateRule(i, 'rewrite_message', e.target.value)
                }
                className="h-7 text-xs"
              />
            </div>
          )}
          {rule.action === 'replace' && (
            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-1">
                <Label className="text-xs">{t('Replace Status Code')}</Label>
                <Input
                  type="number"
                  value={rule.replace_status_code}
                  onChange={(e) =>
                    updateRule(
                      i,
                      'replace_status_code',
                      Number(e.target.value)
                    )
                  }
                  className="h-7 text-xs"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">{t('Replace Message')}</Label>
                <Input
                  value={rule.replace_message}
                  onChange={(e) =>
                    updateRule(i, 'replace_message', e.target.value)
                  }
                  className="h-7 text-xs"
                />
              </div>
            </div>
          )}
        </div>
      ))}
      {rules.length === 0 && (
        <p className="text-muted-foreground text-center text-xs py-2">
          {t('No rules configured')}
        </p>
      )}
      <Separator />
    </div>
  )
}
