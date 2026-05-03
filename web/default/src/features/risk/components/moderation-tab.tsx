import { useState, useMemo, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Play, Eye } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  getModerationConfig,
  saveModerationConfig,
  getModerationOverview,
  getModerationRules,
  getModerationCategories,
  getModerationIncidents,
  getModerationIncidentDetail,
  deleteModerationRule,
  getModerationQueueStats,
  runModerationDebug,
  getModerationDebugResult,
  type ModerationConfig,
  type ModerationRule,
} from '../api'
import {
  riskQueryKeys,
} from '../constants'
import { ModerationRuleEditorDialog } from './moderation/moderation-rule-editor'

export function ModerationTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [ruleEditorOpen, setRuleEditorOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<ModerationRule | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deletingRule, setDeletingRule] = useState<ModerationRule | null>(null)
  const [incidentPage, setIncidentPage] = useState(1)
  const [incidentFilters, setIncidentFilters] = useState({
    group: '__all__',
    flagged: '__all__',
    keyword: '',
  })
  const [debugText, setDebugText] = useState('')
  const [debugGroup, setDebugGroup] = useState('__default__')
  const [debugRunning, setDebugRunning] = useState(false)
  const [debugResult, setDebugResult] = useState<Record<string, unknown> | null>(null)
  const [detailId, setDetailId] = useState<number | null>(null)

  const { data: config } = useQuery({
    queryKey: riskQueryKeys.moderation.config(),
    queryFn: getModerationConfig,
  })

  const { data: overview } = useQuery({
    queryKey: riskQueryKeys.moderation.overview(),
    queryFn: getModerationOverview,
  })

  const { data: rules } = useQuery({
    queryKey: riskQueryKeys.moderation.rules(),
    queryFn: getModerationRules,
  })

  const { data: categories } = useQuery({
    queryKey: riskQueryKeys.moderation.categories(),
    queryFn: getModerationCategories,
  })

  const { data: queueStats } = useQuery({
    queryKey: riskQueryKeys.moderation.queueStats(),
    queryFn: getModerationQueueStats,
    refetchInterval: 15000,
  })

  const incidentParams = useMemo(
    () => ({
      p: incidentPage,
      page_size: 10,
      group: incidentFilters.group === '__all__' ? undefined : incidentFilters.group,
      flagged: incidentFilters.flagged === '__all__' ? undefined : incidentFilters.flagged,
      keyword: incidentFilters.keyword || undefined,
    }),
    [incidentPage, incidentFilters]
  )

  const { data: incidentsData, isLoading: incidentsLoading } = useQuery({
    queryKey: riskQueryKeys.moderation.incidents(incidentParams),
    queryFn: () => getModerationIncidents(incidentParams),
    placeholderData: (prev) => prev,
  })

  const incidents = incidentsData?.items ?? []
  const incidentTotal = incidentsData?.total ?? 0

  const { data: detailData } = useQuery({
    queryKey: riskQueryKeys.moderation.incidentDetail(detailId!),
    queryFn: () => getModerationIncidentDetail(detailId!),
    enabled: detailId !== null,
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

  const [localConfig, setLocalConfig] = useState<ModerationConfig | null>(null)
  useEffect(() => {
    if (config) setLocalConfig(config.config)
  }, [config])

  const configMutation = useMutation({
    mutationFn: (cfg: Partial<ModerationConfig>) =>
      saveModerationConfig(cfg),
    onSuccess: () => {
      toast.success(t('Config saved'))
      queryClient.invalidateQueries({
        queryKey: riskQueryKeys.moderation.all,
      })
    },
  })

  const deleteRuleMutation = useMutation({
    mutationFn: (id: number) => deleteModerationRule(id),
    onSuccess: () => {
      toast.success(t('Rule deleted'))
      setDeleteOpen(false)
      queryClient.invalidateQueries({
        queryKey: riskQueryKeys.moderation.rules(),
      })
    },
  })

  const handleDebug = useCallback(async () => {
    if (!debugText.trim()) return
    setDebugRunning(true)
    setDebugResult(null)
    try {
      const { request_id } = await runModerationDebug({
        text: debugText,
        group: debugGroup === '__default__' ? undefined : debugGroup,
      })
      let attempts = 0
      const poll = async (): Promise<void> => {
        if (attempts >= 30) {
          setDebugResult({ error: t('Debug timeout') })
          setDebugRunning(false)
          return
        }
        attempts++
        const result = await getModerationDebugResult(request_id)
        if (!result.pending || result.result?.error) {
          setDebugResult(result as unknown as Record<string, unknown>)
          setDebugRunning(false)
          return
        }
        await new Promise((r) => setTimeout(r, 1000))
        return poll()
      }
      await poll()
    } catch {
      toast.error(t('Debug failed'))
      setDebugRunning(false)
    }
  }, [debugText, debugGroup, t])

  const groupNames = useMemo(() => {
    return config?.config?.enabled_groups ?? []
  }, [config])

  return (
    <div className="space-y-6">
      <Alert>
        <AlertDescription>
          {t(
            'Content moderation is off by default and requires explicit configuration.'
          )}
        </AlertDescription>
      </Alert>

      {/* Queue Stats */}
      {queueStats && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('Runtime Stats')}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
              <div>
                <span className="text-muted-foreground">{t('Memory Queue')}: </span>
                {queueStats.queue_depth_memory}
              </div>
              <div>
                <span className="text-muted-foreground">{t('Redis Queue')}: </span>
                {queueStats.queue_depth_redis}
              </div>
              <div>
                <span className="text-muted-foreground">{t('Workers')}: </span>
                {queueStats.worker_count}
              </div>
              <div>
                <span className="text-muted-foreground">{t('Drops')}: </span>
                {queueStats.drop_count_total}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Overview Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <OverviewCard
          title={t('Status')}
          value={overview?.enabled ? t('Enabled') : t('Disabled')}
          extra={t('Global mode: ') + (overview?.mode ?? '-')}
        />
        <OverviewCard
          title={t('API Keys')}
          value={overview?.key_count ?? 0}
          extra={t('Multi-key rotation with auto-failover')}
        />
        <OverviewCard
          title={t('Flagged (24h)')}
          value={overview?.flagged_24h ?? 0}
          extra={`${t('Rules')}: ${overview?.rule_count ?? 0} / ${t('Enabled groups')}: ${overview?.enabled_group_count ?? 0}`}
        />
        <OverviewCard
          title={t('Event Drops')}
          value={overview?.queue_dropped ?? 0}
          extra={`${t('Sampling rate')}: ${overview?.sampling_rate_percent ?? 0}%`}
        />
      </div>

      {/* Config Panel */}
      {localConfig && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Moderation Config')}</CardTitle>
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
              <div className="space-y-1">
                <Label>{t('Mode')}</Label>
                <Select
                  value={localConfig.mode}
                  onValueChange={(v) =>
                    setLocalConfig((p) => p && { ...p, mode: v })
                  }
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="enforce">{t('Enforce')}</SelectItem>
                    <SelectItem value="observe_only">
                      {t('Observe Only')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label>{t('Base URL')}</Label>
                <Input
                  value={localConfig.base_url}
                  onChange={(e) =>
                    setLocalConfig((p) => p && { ...p, base_url: e.target.value })
                  }
                  placeholder="https://api.openai.com"
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Model')}</Label>
                <Input
                  value={localConfig.model}
                  onChange={(e) =>
                    setLocalConfig((p) => p && { ...p, model: e.target.value })
                  }
                  placeholder="omni-moderation-latest"
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Sampling Rate')}</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  max="1"
                  value={localConfig.sampling_rate_percent}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, sampling_rate_percent: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Workers')}</Label>
                <Input
                  type="number"
                  value={localConfig.worker_count}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, worker_count: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Queue Size')}</Label>
                <Input
                  type="number"
                  value={localConfig.event_queue_size}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, event_queue_size: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Retention Hours')}</Label>
                <Input
                  type="number"
                  value={localConfig.flagged_retention_hours}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, flagged_retention_hours: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('HTTP Timeout (ms)')}</Label>
                <Input
                  type="number"
                  value={localConfig.http_timeout_ms}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, http_timeout_ms: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Max Retries')}</Label>
                <Input
                  type="number"
                  value={localConfig.max_retries}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, max_retries: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="flex items-center gap-3">
                <Label>{t('Record Unmatched')}</Label>
                <Switch
                  checked={localConfig.record_unmatched_inputs}
                  onCheckedChange={(v) =>
                    setLocalConfig((p) => p && { ...p, record_unmatched_inputs: v })
                  }
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label>{t('API Keys')}</Label>
              <Textarea
                value={(localConfig.api_keys ?? []).join('\n')}
                onChange={(e) =>
                  setLocalConfig((p) =>
                    p && { ...p, api_keys: e.target.value.split('\n').filter(Boolean) }
                  )
                }
                rows={3}
                placeholder={t('One key per line')}
              />
            </div>
            <Separator />
            <Button
              size="sm"
              onClick={() => localConfig && configMutation.mutate(localConfig)}
              disabled={configMutation.isPending}
            >
              {t('Save')}
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Rules */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>{t('Moderation Rules')}</CardTitle>
          <Button
            size="sm"
            onClick={() => {
              setEditingRule(null)
              setRuleEditorOpen(true)
            }}
          >
            <Plus className="mr-1 h-3.5 w-3.5" />
            {t('Add Rule')}
          </Button>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Enabled')}</TableHead>
                  <TableHead>{t('Name')}</TableHead>
                  <TableHead>{t('Logic')}</TableHead>
                  <TableHead>{t('Action')}</TableHead>
                  <TableHead>{t('Priority')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(rules ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center py-6 text-muted-foreground">
                      {t('No rules configured')}
                    </TableCell>
                  </TableRow>
                ) : (
                  (rules ?? []).map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell>
                        <StatusBadge
                          variant={rule.enabled ? 'success' : 'neutral'}
                        >
                          {rule.enabled ? t('On') : t('Off')}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="font-medium">
                        {rule.name}
                      </TableCell>
                      <TableCell className="uppercase text-xs">
                        {rule.match_mode}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          variant={
                            rule.action === 'block' ? 'danger' : 'warning'
                          }
                        >
                          {rule.action}
                        </StatusBadge>
                      </TableCell>
                      <TableCell>{rule.priority}</TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => {
                              setEditingRule(rule)
                              setRuleEditorOpen(true)
                            }}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => {
                              setDeletingRule(rule)
                              setDeleteOpen(true)
                            }}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      {/* Debug */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Moderation Debug')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_150px_auto]">
            <Textarea
              value={debugText}
              onChange={(e) => setDebugText(e.target.value)}
              placeholder={t('Enter text to test...')}
              rows={2}
            />
            <Select value={debugGroup} onValueChange={setDebugGroup}>
              <SelectTrigger className="h-8">
                <SelectValue placeholder={t('Group')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__default__">{t('Default')}</SelectItem>
                {groupNames.map((g) => (
                  <SelectItem key={g} value={g}>
                    {g}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              onClick={handleDebug}
              disabled={debugRunning || !debugText.trim()}
            >
              <Play className="mr-1 h-3.5 w-3.5" />
              {debugRunning ? t('Running...') : t('Run Debug')}
            </Button>
          </div>
          {debugResult && (
            <pre className="bg-muted rounded-md p-3 text-xs overflow-auto max-h-[200px]">
              {JSON.stringify(debugResult, null, 2)}
            </pre>
          )}
        </CardContent>
      </Card>

      {/* Incidents */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Moderation Incidents')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={incidentFilters.group}
              onValueChange={(v) =>
                setIncidentFilters((p) => ({ ...p, group: v }))
              }
            >
              <SelectTrigger className="h-8 w-[130px]">
                <SelectValue placeholder={t('Group')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">{t('All')}</SelectItem>
                {groupNames.map((g) => (
                  <SelectItem key={g} value={g}>
                    {g}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={incidentFilters.flagged}
              onValueChange={(v) =>
                setIncidentFilters((p) => ({ ...p, flagged: v }))
              }
            >
              <SelectTrigger className="h-8 w-[120px]">
                <SelectValue placeholder={t('Flagged')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">{t('All')}</SelectItem>
                <SelectItem value="true">{t('Flagged')}</SelectItem>
                <SelectItem value="false">{t('Clean')}</SelectItem>
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
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead>{t('Input Summary')}</TableHead>
                  <TableHead>{t('Flagged')}</TableHead>
                  <TableHead>{t('Action')}</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {incidentsLoading && incidents.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={8} className="text-center py-8">
                      {t('Loading...')}
                    </TableCell>
                  </TableRow>
                ) : incidents.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={8}
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
                        UID {item.user_id}
                      </TableCell>
                      <TableCell className="text-xs">
                        {item.model}
                      </TableCell>
                      <TableCell>
                        <StatusBadge variant="cyan">
                          {item.group || '-'}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="max-w-[200px] truncate text-xs">
                        {item.input_summary ? (
                          <button
                            type="button"
                            className="text-left text-primary hover:underline cursor-pointer truncate block max-w-full"
                            onClick={() => setDetailId(item.id)}
                          >
                            {item.input_summary}
                          </button>
                        ) : (
                          '-'
                        )}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          variant={item.flagged ? 'danger' : 'success'}
                        >
                          {item.flagged ? t('Flagged') : t('Clean')}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="text-xs">
                        {item.decision || '-'}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => setDetailId(item.id)}
                        >
                          <Eye className="h-3.5 w-3.5" />
                        </Button>
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

      {/* Dialogs */}
      <ModerationRuleEditorDialog
        open={ruleEditorOpen}
        onOpenChange={setRuleEditorOpen}
        initialRule={editingRule}
        categories={categories ?? []}
        onSaved={() => {
          setRuleEditorOpen(false)
          queryClient.invalidateQueries({
            queryKey: riskQueryKeys.moderation.rules(),
          })
        }}
      />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete Rule')}
        desc={`${t('Rule')}: ${deletingRule?.name || '-'}`}
        handleConfirm={() =>
          deletingRule && deleteRuleMutation.mutate(deletingRule.id)
        }
        isLoading={deleteRuleMutation.isPending}
      />

      {/* Incident Detail Dialog */}
      <Dialog open={detailId !== null} onOpenChange={(open) => !open && setDetailId(null)}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t('Incident Detail')}</DialogTitle>
          </DialogHeader>
          {detailData ? (
            <div className="space-y-4">
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted-foreground">{t('Request ID')}</dt>
                <dd className="font-mono text-xs break-all">{detailData.request_id}</dd>
                <dt className="text-muted-foreground">{t('User')}</dt>
                <dd>{detailData.username} (UID {detailData.user_id})</dd>
                <dt className="text-muted-foreground">{t('Token')}</dt>
                <dd>ID {detailData.token_id}</dd>
                <dt className="text-muted-foreground">{t('Model')}</dt>
                <dd>{detailData.model}</dd>
                <dt className="text-muted-foreground">{t('Group')}</dt>
                <dd>
                  <StatusBadge variant="cyan">{detailData.group || '-'}</StatusBadge>
                </dd>
                <dt className="text-muted-foreground">{t('Decision')}</dt>
                <dd>{detailData.decision || '-'}</dd>
                <dt className="text-muted-foreground">{t('Flagged')}</dt>
                <dd>
                  <StatusBadge variant={detailData.flagged ? 'danger' : 'success'}>
                    {detailData.flagged ? t('Flagged') : t('Clean')}
                  </StatusBadge>
                </dd>
                <dt className="text-muted-foreground">{t('Max Category')}</dt>
                <dd>{detailData.max_category || '-'}</dd>
                <dt className="text-muted-foreground">{t('Max Score')}</dt>
                <dd>{detailData.max_score ?? '-'}</dd>
                <dt className="text-muted-foreground">{t('Matched Rules')}</dt>
                <dd>{detailData.matched_rules || '-'}</dd>
                <dt className="text-muted-foreground">{t('Time')}</dt>
                <dd>{formatTimestamp(detailData.created_at)}</dd>
              </dl>
              {detailData.categories && (
                <>
                  <Separator />
                  <div>
                    <Label className="text-xs font-medium">{t('Category Scores')}</Label>
                    <pre className="bg-muted mt-1 rounded-md p-3 text-xs overflow-auto max-h-[150px]">
                      {typeof detailData.categories === 'string'
                        ? (() => { try { return JSON.stringify(JSON.parse(detailData.categories), null, 2) } catch { return detailData.categories } })()
                        : JSON.stringify(detailData.categories, null, 2)}
                    </pre>
                  </div>
                </>
              )}
              {detailData.input_summary && (
                <>
                  <Separator />
                  <div>
                    <Label className="text-xs font-medium">{t('Input Content')}</Label>
                    <pre className="bg-muted mt-1 rounded-md p-3 text-xs overflow-auto max-h-[200px] whitespace-pre-wrap break-words">
                      {detailData.input_summary}
                    </pre>
                  </div>
                </>
              )}
            </div>
          ) : (
            <div className="text-center py-8 text-muted-foreground text-sm">
              {t('Loading...')}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
