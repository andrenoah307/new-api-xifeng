import { useState, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, RefreshCw } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  getRiskOverview,
  getRiskConfig,
  saveRiskConfig,
  getRiskRules,
  getRiskGroups,
  type RiskConfig,
  type RiskRule,
} from '../api'
import {
  riskQueryKeys,
  safeParseJSON,
} from '../constants'
import { OverviewCard } from './overview/overview-card'
import { RiskConfigPanel } from './config/risk-config-panel'
import { RiskGroupsPanel } from './groups/risk-groups-panel'
import { RiskSubjectsTable } from './subjects/risk-subjects-table'
import { RiskIncidentsTable } from './incidents/risk-incidents-table'
import { RiskRulesTable } from './rules/risk-rules-table'
import { RuleEditorDialog } from './rules/rule-editor-dialog'

export function DistributionTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<RiskRule | null>(null)

  const { data: overview } = useQuery({
    queryKey: riskQueryKeys.overview(),
    queryFn: getRiskOverview,
  })

  const { data: config } = useQuery({
    queryKey: riskQueryKeys.config(),
    queryFn: getRiskConfig,
  })

  const { data: rules } = useQuery({
    queryKey: riskQueryKeys.rules(),
    queryFn: getRiskRules,
  })

  const { data: groups } = useQuery({
    queryKey: riskQueryKeys.groups(),
    queryFn: getRiskGroups,
  })

  const enabledGroupSet = useMemo(() => {
    const set = new Set<string>()
    for (const g of groups ?? []) {
      if (g.enabled) set.add(g.name)
    }
    return set
  }, [groups])

  const groupOptions = useMemo(
    () => (groups ?? []).map((g) => g.name),
    [groups]
  )

  const saveMutation = useMutation({
    mutationFn: (cfg: Partial<RiskConfig>) => saveRiskConfig(cfg),
    onSuccess: () => {
      toast.success(t('Config saved'))
      queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
    },
  })

  const handleEditRule = useCallback((rule: RiskRule) => {
    setEditingRule(rule)
    setEditorOpen(true)
  }, [])

  const handleCreateRule = useCallback(() => {
    setEditingRule(null)
    setEditorOpen(true)
  }, [])

  const handleRuleSaved = useCallback(() => {
    setEditorOpen(false)
    setEditingRule(null)
    queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
  }, [queryClient])

  const refreshAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
  }, [queryClient])

  return (
    <div className="space-y-6">
      {config && !config.async_event_engine && (
        <Alert>
          <AlertDescription>
            {t(
              'Async event engine is not enabled. Some features may be limited.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Overview Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
        <OverviewCard
          title={t('Observed Subjects')}
          value={overview?.observed_subjects ?? 0}
        />
        <OverviewCard
          title={t('Blocked Subjects')}
          value={overview?.blocked_subjects ?? 0}
        />
        <OverviewCard
          title={t('High Risk Subjects')}
          value={overview?.high_risk_subjects ?? 0}
        />
        <OverviewCard
          title={t('Total Rules')}
          value={overview?.rule_count ?? 0}
        />
        <OverviewCard
          title={t('Enabled Groups')}
          value={overview?.enabled_group_count ?? 0}
        />
        <OverviewCard
          title={t('Unconfigured Rules')}
          value={overview?.unconfigured_rule_count ?? 0}
        />
        <OverviewCard
          title={t('Unlisted Rules')}
          value={overview?.group_unlisted_rule_count ?? 0}
        />
      </div>

      {/* Global Config */}
      {config && (
        <RiskConfigPanel
          config={config}
          saving={saveMutation.isPending}
          onSave={(cfg) => saveMutation.mutate(cfg)}
        />
      )}

      {/* Group Enable Matrix */}
      {groups && config && (
        <RiskGroupsPanel
          groups={groups}
          config={config}
          onConfigChange={(cfg) => saveMutation.mutate(cfg)}
          saving={saveMutation.isPending}
        />
      )}

      {/* Inner tabs: Subjects / Incidents / Rules */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">{t('Detection Data')}</h3>
        <Button variant="outline" size="sm" onClick={refreshAll}>
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          {t('Refresh')}
        </Button>
      </div>

      <Tabs defaultValue="subjects">
        <TabsList>
          <TabsTrigger value="subjects">{t('Risk Subjects')}</TabsTrigger>
          <TabsTrigger value="incidents">{t('Risk Incidents')}</TabsTrigger>
          <TabsTrigger value="rules">
            {t('Risk Rules')}
            <Button
              variant="ghost"
              size="icon"
              className="ml-1 h-5 w-5"
              onClick={(e) => {
                e.stopPropagation()
                handleCreateRule()
              }}
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </TabsTrigger>
        </TabsList>
        <TabsContent value="subjects" className="mt-3">
          <RiskSubjectsTable />
        </TabsContent>
        <TabsContent value="incidents" className="mt-3">
          <RiskIncidentsTable />
        </TabsContent>
        <TabsContent value="rules" className="mt-3">
          <RiskRulesTable
            rules={rules ?? []}
            onEdit={handleEditRule}
            enabledGroupSet={enabledGroupSet}
          />
        </TabsContent>
      </Tabs>

      <RuleEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        initialRule={editingRule}
        groupOptions={groupOptions}
        enabledGroupSet={enabledGroupSet}
        onSaved={handleRuleSaved}
      />
    </div>
  )
}
