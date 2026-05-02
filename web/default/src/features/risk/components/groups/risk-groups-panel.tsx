import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { StatusBadge } from '@/components/status-badge'
import {
  Select,
  SelectContent,
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

  const enabledGroups = new Set(
    Array.isArray((config as Record<string, unknown>).enabled_groups)
      ? ((config as Record<string, unknown>).enabled_groups as string[])
      : []
  )
  const groupModes: Record<string, string> =
    ((config as Record<string, unknown>).group_modes as Record<string, string>) ?? {}

  const toggleGroup = (name: string, enabled: boolean) => {
    const list = [...enabledGroups]
    const next = list.filter((g) => g !== name)
    if (enabled) next.push(name)
    onConfigChange({
      ...config,
      ...(({ enabled_groups: next }) as Partial<RiskConfig>),
    } as Partial<RiskConfig>)
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
      ...(({ group_modes: modes }) as Partial<RiskConfig>),
    } as Partial<RiskConfig>)
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
                      value={groupModes[g.name] ?? '__inherit__'}
                      onValueChange={(v) => setGroupMode(g.name, v)}
                    >
                      <SelectTrigger className="h-7 w-[140px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="__inherit__">
                          {t('Inherit Global')}
                        </SelectItem>
                        <SelectItem value="enforce">
                          {t('Enforce')}
                        </SelectItem>
                        <SelectItem value="observe_only">
                          {t('Observe Only')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      variant={
                        g.effective_mode === 'enforce'
                          ? 'danger'
                          : g.effective_mode === 'observe_only'
                            ? 'warning'
                            : 'neutral'
                      }
                    >
                      {g.effective_mode || config.mode}
                    </StatusBadge>
                  </TableCell>
                  <TableCell>{g.rule_count}</TableCell>
                  <TableCell>{g.subject_count}</TableCell>
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
