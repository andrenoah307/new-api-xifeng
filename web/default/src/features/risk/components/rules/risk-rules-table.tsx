import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Pencil, Trash2 } from 'lucide-react'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useState } from 'react'
import type { RiskRule } from '../../api'
import { deleteRiskRule, updateRiskRule } from '../../api'
import {
  riskQueryKeys,
  safeParseJSON,
  METRIC_LABEL_MAP,
} from '../../constants'

interface Props {
  rules: RiskRule[]
  onEdit: (rule: RiskRule) => void
  enabledGroupSet: Set<string>
}

export function RiskRulesTable({ rules, onEdit, enabledGroupSet }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deletingRule, setDeletingRule] = useState<RiskRule | null>(null)

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteRiskRule(id),
    onSuccess: () => {
      toast.success(t('Rule deleted'))
      setDeleteOpen(false)
      queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({
      rule,
      enabled,
    }: {
      rule: RiskRule
      enabled: boolean
    }) => {
      const groups = safeParseJSON<string[]>(rule.groups, [])
      if (enabled && groups.length === 0) {
        throw new Error(t('Rule must be bound to at least one group before enabling'))
      }
      const conditions = safeParseJSON(rule.conditions, [])
      return updateRiskRule(rule.id, { ...rule, conditions, groups, enabled })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : String(err))
    },
  })

  const formatConditions = useCallback(
    (rule: RiskRule) => {
      const conds = safeParseJSON(rule.conditions, []) as Array<{
        metric: string
        op: string
        value: number
      }>
      return conds
        .map((c) => `${t(METRIC_LABEL_MAP[c.metric] ?? c.metric)} ${c.op} ${c.value}`)
        .join(rule.match_mode === 'all' ? ' AND ' : ' OR ')
    },
    [t]
  )

  return (
    <>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Enabled')}</TableHead>
              <TableHead>{t('Name')}</TableHead>
              <TableHead>{t('Scope')}</TableHead>
              <TableHead>{t('Conditions')}</TableHead>
              <TableHead>{t('Action')}</TableHead>
              <TableHead>{t('Groups')}</TableHead>
              <TableHead>{t('Priority')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rules.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="text-center py-8 text-muted-foreground">
                  {t('No rules configured')}
                </TableCell>
              </TableRow>
            ) : (
              rules.map((rule) => {
                const groups = safeParseJSON<string[]>(rule.groups, [])
                return (
                  <TableRow key={rule.id}>
                    <TableCell>
                      <Switch
                        checked={rule.enabled}
                        onCheckedChange={(v) =>
                          toggleMutation.mutate({ rule, enabled: v })
                        }
                      />
                    </TableCell>
                    <TableCell className="font-medium">
                      {rule.name}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        variant={
                          rule.scope === 'token' ? 'blue' : 'success'
                        }
                      >
                        {rule.scope === 'token' ? 'Token' : t('User')}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="max-w-[300px] truncate text-xs">
                      {formatConditions(rule)}
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
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {groups.map((g) => (
                          <StatusBadge
                            key={g}
                            variant={
                              enabledGroupSet.has(g) ? 'cyan' : 'neutral'
                            }
                          >
                            {g}
                          </StatusBadge>
                        ))}
                        {groups.length === 0 && (
                          <span className="text-muted-foreground text-xs">
                            -
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>{rule.priority}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => onEdit(rule)}
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
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete Rule')}
        desc={`${t('Rule')}: ${deletingRule?.name || '-'}`}
        handleConfirm={() =>
          deletingRule && deleteMutation.mutate(deletingRule.id)
        }
        isLoading={deleteMutation.isPending}
      />
    </>
  )
}
