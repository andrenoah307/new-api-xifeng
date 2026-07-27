import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { MODE_OPTIONS, optionLabelKey } from '../../constants'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { StatusBadge } from '@/components/status-badge'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { RiskGroup, RiskConfig } from '../../api'

interface Props {
  groups: RiskGroup[]
  config: RiskConfig
  onConfigChange: (config: Partial<RiskConfig>) => void
  saving: boolean
}

export function RiskGroupsPanel({
  groups,
  config,
  onConfigChange,
  saving,
}: Props) {
  const { t } = useTranslation()

  const modeItems = useMemo(
    () => [
      { value: '__inherit__', label: t('Inherit Global') },
      { value: 'enforce', label: t('Enforce') },
      { value: 'observe_only', label: t('Observe Only') },
    ],
    [t]
  )

  const enabledGroups = new Set(
    Array.isArray(config.enabled_groups)
      ? config.enabled_groups
      : []
  )
  const groupModes: Record<string, string> = config.group_modes ?? {}

  const toggleGroup = (name: string, enabled: boolean) => {
    const list = [...enabledGroups]
    const next = list.filter((g) => g !== name)
    if (enabled) next.push(name)
    onConfigChange({
      ...config,
      enabled_groups: next,
    })
  }

  const setGroupMode = (name: string, mode: string) => {
    const modes = { ...groupModes }
    if (mode === '__inherit__') {
      delete modes[name]
    } else {
      modes[name] = mode
    }
    onConfigChange({
      ...config,
      group_modes: modes,
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Group Enable Matrix')}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Group')}</TableHead>
                <TableHead>{t('Enabled')}</TableHead>
                <TableHead>{t('Mode Override')}</TableHead>
                <TableHead>{t('Effective Mode')}</TableHead>
                <TableHead>{t('Rules')}</TableHead>
                <TableHead>{t('Subjects')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((g) => (
                <TableRow key={g.name}>
                  <TableCell className="font-medium">{g.name}</TableCell>
                  <TableCell>
                    <Switch
                      checked={enabledGroups.has(g.name)}
                      onCheckedChange={(v) => toggleGroup(g.name, v)}
                    />
                  </TableCell>
                  <TableCell>
                    <Select
                      items={modeItems}
                      value={groupModes[g.name] ?? '__inherit__'}
                      onValueChange={(v) => setGroupMode(g.name, v)}
                    >
                      <SelectTrigger className="h-7 w-[140px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value="__inherit__">
                            {t('Inherit Global')}
                          </SelectItem>
                          <SelectItem value="enforce">
                            {t('Enforce')}
                          </SelectItem>
                          <SelectItem value="observe_only">
                            {t('Observe Only')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      copyable={false}
                      variant={
                        g.effective_mode === 'enforce'
                          ? 'danger'
                          : g.effective_mode === 'observe_only'
                            ? 'warning'
                            : 'neutral'
                      }
                    >
                      {t(
                        optionLabelKey(
                          MODE_OPTIONS,
                          g.effective_mode || config.mode
                        )
                      )}
                    </StatusBadge>
                  </TableCell>
                  <TableCell>{g.rule_count_total}</TableCell>
                  <TableCell>{g.active_subject_count}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <Button
          size="sm"
          className="mt-3"
          onClick={() => onConfigChange(config)}
          disabled={saving}
        >
          {t('Save')}
        </Button>
      </CardContent>
    </Card>
  )
}
