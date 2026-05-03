import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import type { RiskConfig } from '../../api'
import { MODE_OPTIONS } from '../../constants'

interface Props {
  config: RiskConfig
  saving: boolean
  onSave: (config: Partial<RiskConfig>) => void
}

export function RiskConfigPanel({ config, saving, onSave }: Props) {
  const { t } = useTranslation()
  const [local, setLocal] = useState(config)

  useEffect(() => {
    setLocal(config)
  }, [config])

  const update = (field: string, value: unknown) => {
    setLocal((prev) => ({ ...prev, [field]: value }))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Global Policy')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div className="flex items-center gap-3">
            <Label>{t('Enabled')}</Label>
            <Switch
              checked={local.enabled}
              onCheckedChange={(v) => update('enabled', v)}
            />
          </div>
          <div className="space-y-1">
            <Label>{t('Mode')}</Label>
            <Select
              value={local.mode}
              onValueChange={(v) => update('mode', v)}
            >
              <SelectTrigger className="h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {MODE_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {t(o.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>{t('Default Status Code')}</Label>
            <Input
              type="number"
              value={local.default_status_code}
              onChange={(e) =>
                update('default_status_code', Number(e.target.value))
              }
              className="h-8"
            />
          </div>
          <div className="space-y-1">
            <Label>{t('Default Recovery Seconds')}</Label>
            <Input
              type="number"
              value={local.default_recover_after_secs}
              onChange={(e) =>
                update(
                  'default_recover_after_secs',
                  Number(e.target.value)
                )
              }
              className="h-8"
            />
          </div>
          <div className="space-y-1">
            <Label>{t('Default Response Message')}</Label>
            <Input
              value={local.default_response_message}
              onChange={(e) =>
                update('default_response_message', e.target.value)
              }
              className="h-8"
            />
          </div>
          <div className="space-y-1">
            <Label>{t('Trusted IP Header')}</Label>
            <Input
              value={local.trusted_ip_header ?? ''}
              onChange={(e) => update('trusted_ip_header', e.target.value)}
              placeholder="X-Forwarded-For"
              className="h-8"
            />
          </div>
        </div>
        <Separator />
        <Button
          size="sm"
          onClick={() => onSave(local)}
          disabled={saving}
        >
          {t('Save')}
        </Button>
      </CardContent>
    </Card>
  )
}
