import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Mail } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Separator } from '@/components/ui/separator'
import { StatusBadge } from '@/components/status-badge'
import { ConfirmDialog } from '@/components/confirm-dialog'
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
import { DataTablePagination } from '@/components/data-table/pagination'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { formatTimestamp } from '@/lib/format'
import { OverviewCard } from './overview/overview-card'
import {
  getEnforcementConfig,
  saveEnforcementConfig,
  getEnforcementOverview,
  getEnforcementCounters,
  getEnforcementIncidents,
  resetEnforcementCounter,
  unbanUser,
  sendTestEmail,
  type EnforcementConfig,
} from '../api'
import {
  riskQueryKeys,
  ENFORCEMENT_SOURCE_OPTIONS,
  ENFORCEMENT_ACTION_OPTIONS,
} from '../constants'

export function EnforcementTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [counterPage, setCounterPage] = useState(1)
  const [incidentPage, setIncidentPage] = useState(1)
  const [incidentFilters, setIncidentFilters] = useState({
    source: '__all__',
    action: '__all__',
    keyword: '',
  })
  const [resetOpen, setResetOpen] = useState(false)
  const [unbanOpen, setUnbanOpen] = useState(false)
  const [targetUid, setTargetUid] = useState(0)

  const { data: config } = useQuery({
    queryKey: riskQueryKeys.enforcement.config(),
    queryFn: getEnforcementConfig,
  })

  const { data: overview } = useQuery({
    queryKey: riskQueryKeys.enforcement.overview(),
    queryFn: getEnforcementOverview,
  })

  const counterParams = useMemo(
    () => ({ p: counterPage, page_size: 10 }),
    [counterPage]
  )

  const { data: countersData, isLoading: countersLoading } = useQuery({
    queryKey: riskQueryKeys.enforcement.counters(counterParams),
    queryFn: () => getEnforcementCounters(counterParams),
    placeholderData: (prev) => prev,
  })

  const counters = countersData?.items ?? []
  const counterTotal = countersData?.total ?? 0

  const incidentParams = useMemo(
    () => ({
      p: incidentPage,
      page_size: 10,
      source: incidentFilters.source === '__all__' ? undefined : incidentFilters.source,
      action: incidentFilters.action === '__all__' ? undefined : incidentFilters.action,
      keyword: incidentFilters.keyword || undefined,
    }),
    [incidentPage, incidentFilters]
  )

  const { data: incidentsData, isLoading: incidentsLoading } = useQuery({
    queryKey: riskQueryKeys.enforcement.incidents(incidentParams),
    queryFn: () => getEnforcementIncidents(incidentParams),
    placeholderData: (prev) => prev,
  })

  const incidents = incidentsData?.items ?? []
  const incidentTotal = incidentsData?.total ?? 0

  const counterTable = useReactTable({
    data: counters,
    columns: [],
    pageCount: Math.ceil(counterTotal / 10),
    state: { pagination: { pageIndex: counterPage - 1, pageSize: 10 } },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: counterPage - 1, pageSize: 10 })
          : updater
      setCounterPage(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const incidentTable = useReactTable({
    data: incidents,
    columns: [],
    pageCount: Math.ceil(incidentTotal / 10),
    state: { pagination: { pageIndex: incidentPage - 1, pageSize: 10 } },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: incidentPage - 1, pageSize: 10 })
          : updater
      setIncidentPage(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const [localConfig, setLocalConfig] = useState<EnforcementConfig | null>(null)
  useEffect(() => {
    if (config) setLocalConfig(config)
  }, [config])

  const configMutation = useMutation({
    mutationFn: (cfg: Partial<EnforcementConfig>) =>
      saveEnforcementConfig(cfg),
    onSuccess: () => {
      toast.success(t('Config saved'))
      queryClient.invalidateQueries({
        queryKey: riskQueryKeys.enforcement.all,
      })
    },
  })

  const resetMutation = useMutation({
    mutationFn: (uid: number) => resetEnforcementCounter(uid),
    onSuccess: () => {
      toast.success(t('Reset Counter'))
      setResetOpen(false)
      queryClient.invalidateQueries({
        queryKey: riskQueryKeys.enforcement.all,
      })
    },
  })

  const unbanMutation = useMutation({
    mutationFn: (uid: number) => unbanUser(uid),
    onSuccess: () => {
      toast.success(t('Unban User'))
      setUnbanOpen(false)
      queryClient.invalidateQueries({
        queryKey: riskQueryKeys.enforcement.all,
      })
    },
  })

  const testEmailMutation = useMutation({
    mutationFn: sendTestEmail,
    onSuccess: () => toast.success(t('Test Email')),
  })

  return (
    <div className="space-y-6">
      <Alert>
        <AlertDescription>
          {t(
            'Enforcement handles post-hit actions: email reminders, counter tracking, and auto-banning.'
          )}
        </AlertDescription>
      </Alert>

      {/* Overview */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <OverviewCard
          title={t('Status')}
          value={overview?.enabled ? t('Enabled') : t('Disabled')}
        />
        <OverviewCard
          title={t('Hits (24h)')}
          value={overview?.hits_24h ?? 0}
        />
        <OverviewCard
          title={t('Auto Bans (24h)')}
          value={overview?.auto_bans_24h ?? 0}
        />
        <OverviewCard
          title={t('Email Reminders')}
          value={
            overview?.email_reminder_enabled ? t('Enabled') : t('Disabled')
          }
        />
      </div>

      {/* Config */}
      {localConfig && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Enforcement Configuration')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <div className="flex items-center gap-3">
                <Label>{t('Enabled')}</Label>
                <Switch
                  checked={localConfig.enabled}
                  onCheckedChange={(v) =>
                    setLocalConfig((p) => p && { ...p, enabled: v })
                  }
                />
              </div>
              <div className="flex items-center gap-3">
                <Label>{t('Email Enabled')}</Label>
                <Switch
                  checked={localConfig.email_enabled}
                  onCheckedChange={(v) =>
                    setLocalConfig((p) => p && { ...p, email_enabled: v })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Count Window (hours)')}</Label>
                <Input
                  type="number"
                  value={localConfig.count_window_hours}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        count_window_hours: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Ban Threshold (Default)')}</Label>
                <Input
                  type="number"
                  value={localConfig.ban_threshold_default}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        ban_threshold_default: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Ban Threshold (Distribution)')}</Label>
                <Input
                  type="number"
                  value={localConfig.ban_threshold_risk_distribution}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        ban_threshold_risk_distribution: Number(
                          e.target.value
                        ),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Ban Threshold (Moderation)')}</Label>
                <Input
                  type="number"
                  value={localConfig.ban_threshold_moderation}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        ban_threshold_moderation: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Email Rate Limit (min)')}</Label>
                <Input
                  type="number"
                  value={localConfig.email_rate_limit_minutes}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        email_rate_limit_minutes: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label>{t('Email Subject Template')}</Label>
              <Input
                value={localConfig.email_template_subject}
                onChange={(e) =>
                  setLocalConfig((p) =>
                    p && { ...p, email_template_subject: e.target.value }
                  )
                }
              />
            </div>
            <div className="space-y-1">
              <Label>{t('Email Body Template')}</Label>
              <Textarea
                value={localConfig.email_template_body}
                onChange={(e) =>
                  setLocalConfig((p) =>
                    p && { ...p, email_template_body: e.target.value }
                  )
                }
                rows={4}
              />
            </div>
            <Separator />
            <div className="flex gap-2">
              <Button
                size="sm"
                onClick={() =>
                  localConfig && configMutation.mutate(localConfig)
                }
                disabled={configMutation.isPending}
              >
                {t('Save')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => testEmailMutation.mutate()}
                disabled={testEmailMutation.isPending}
              >
                <Mail className="mr-1 h-3.5 w-3.5" />
                {t('Test Email')}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Counters */}
      <Card>
        <CardHeader>
          <CardTitle>{t('User Counters')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('User')}</TableHead>
                  <TableHead>{t('Total Hits')}</TableHead>
                  <TableHead>{t('Distribution')}</TableHead>
                  <TableHead>{t('Moderation')}</TableHead>
                  <TableHead>{t('Banned')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {countersLoading && counters.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center py-8">
                      {t('Loading...')}
                    </TableCell>
                  </TableRow>
                ) : counters.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="text-center py-8 text-muted-foreground"
                    >
                      {t('No data')}
                    </TableCell>
                  </TableRow>
                ) : (
                  counters.map((c) => (
                    <TableRow key={c.user_id}>
                      <TableCell>
                        <div>
                          <span className="font-medium">
                            {c.username || '-'}
                          </span>
                          <p className="text-muted-foreground text-xs">
                            UID {c.user_id}
                          </p>
                        </div>
                      </TableCell>
                      <TableCell>{c.total_hits}</TableCell>
                      <TableCell>{c.risk_distribution_hits}</TableCell>
                      <TableCell>{c.moderation_hits}</TableCell>
                      <TableCell>
                        <StatusBadge
                          variant={c.banned ? 'danger' : 'success'}
                        >
                          {c.banned ? t('Banned') : t('Normal')}
                        </StatusBadge>
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setTargetUid(c.user_id)
                              setResetOpen(true)
                            }}
                          >
                            {t('Reset')}
                          </Button>
                          {c.banned && (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setTargetUid(c.user_id)
                                setUnbanOpen(true)
                              }}
                            >
                              {t('Unban')}
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <DataTablePagination table={counterTable} />
        </CardContent>
      </Card>

      {/* Incidents */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Enforcement Incidents')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={incidentFilters.source}
              onValueChange={(v) =>
                setIncidentFilters((p) => ({ ...p, source: v }))
              }
            >
              <SelectTrigger className="h-8 w-[150px]">
                <SelectValue placeholder={t('Source')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">{t('All')}</SelectItem>
                {ENFORCEMENT_SOURCE_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {t(o.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={incidentFilters.action}
              onValueChange={(v) =>
                setIncidentFilters((p) => ({ ...p, action: v }))
              }
            >
              <SelectTrigger className="h-8 w-[150px]">
                <SelectValue placeholder={t('Action')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">{t('All')}</SelectItem>
                {ENFORCEMENT_ACTION_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {t(o.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              placeholder={t('Search...')}
              value={incidentFilters.keyword}
              onChange={(e) =>
                setIncidentFilters((p) => ({
                  ...p,
                  keyword: e.target.value,
                }))
              }
              className="h-8 w-[200px]"
            />
          </div>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Time')}</TableHead>
                  <TableHead>{t('User')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Action')}</TableHead>
                  <TableHead>{t('Detail')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {incidentsLoading && incidents.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center py-8">
                      {t('Loading...')}
                    </TableCell>
                  </TableRow>
                ) : incidents.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className="text-center py-8 text-muted-foreground"
                    >
                      {t('No data')}
                    </TableCell>
                  </TableRow>
                ) : (
                  incidents.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="text-xs">
                        {formatTimestamp(item.created_at)}
                      </TableCell>
                      <TableCell>
                        <span className="font-medium">
                          {item.username || '-'}
                        </span>
                        <span className="text-muted-foreground text-xs ml-1">
                          UID {item.user_id}
                        </span>
                      </TableCell>
                      <TableCell>
                        <StatusBadge variant="blue">
                          {item.source}
                        </StatusBadge>
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          variant={
                            item.action === 'auto_ban'
                              ? 'danger'
                              : item.action === 'email'
                                ? 'warning'
                                : 'neutral'
                          }
                        >
                          {item.action}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="max-w-[300px] truncate text-xs">
                        {item.detail || '-'}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <DataTablePagination table={incidentTable} />
        </CardContent>
      </Card>

      <ConfirmDialog
        open={resetOpen}
        onOpenChange={setResetOpen}
        title={t('Reset Counter')}
        desc={t('Reset hit counter for user {{uid}}?', { uid: targetUid })}
        handleConfirm={() => resetMutation.mutate(targetUid)}
        isLoading={resetMutation.isPending}
      />
      <ConfirmDialog
        open={unbanOpen}
        onOpenChange={setUnbanOpen}
        title={t('Unban User')}
        desc={t('Unban user {{uid}} and reset counter?', { uid: targetUid })}
        handleConfirm={() => unbanMutation.mutate(targetUid)}
        isLoading={unbanMutation.isPending}
      />
    </div>
  )
}
