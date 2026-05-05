import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { StatusBadge } from '@/components/status-badge'
import { CopyButton } from '@/components/copy-button'
import { formatTimestampToDate, formatQuota } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Input } from '@/components/ui/input'
import { getUserQuota, type TicketRefund } from '../api'
import { REFUND_STATUS, REFUND_STATUS_CONFIG, PAYEE_TYPE_OPTIONS } from '../constants'

interface RefundDetailProps {
  refund: TicketRefund
  readonly?: boolean
  loading?: boolean
  onStatusChange?: (
    status: number,
    extra?: { quota_mode?: string; actual_refund_quota?: number }
  ) => void
  onSendMessage?: (content: string) => void
}

function CopyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="flex items-center gap-1">
        <span className="font-medium">{value || '-'}</span>
        {value && <CopyButton value={value} size="icon" className="h-6 w-6" iconClassName="h-3 w-3" />}
      </dd>
    </div>
  )
}

function parseQuotaInput(str: string): number | null {
  const trimmed = str.trim()
  if (trimmed === '') return null
  const num = Number(trimmed)
  if (!Number.isFinite(num) || num < 0) return null
  return Math.round(num * 500000)
}

export function RefundDetail({
  refund,
  readonly,
  loading,
  onStatusChange,
  onSendMessage,
}: RefundDetailProps) {
  const { t } = useTranslation()
  const statusCfg = REFUND_STATUS_CONFIG[refund.refund_status]

  const [resolveOpen, setResolveOpen] = useState(false)
  const [rejectOpen, setRejectOpen] = useState(false)
  const [quotaMode, setQuotaMode] = useState('write_off')
  const [customAmount, setCustomAmount] = useState('')
  const [rejectReason, setRejectReason] = useState('')

  useEffect(() => {
    if (resolveOpen) {
      setQuotaMode('write_off')
      setCustomAmount('')
    }
  }, [resolveOpen])

  const { data: targetUser } = useQuery({
    queryKey: ['user', 'quota', refund.user_id],
    queryFn: () => getUserQuota(refund.user_id),
    enabled: resolveOpen && refund.user_id > 0,
  })

  const targetUserQuota = targetUser?.quota ?? null

  const payeeLabel =
    PAYEE_TYPE_OPTIONS.find((o) => o.value === refund.payee_type)?.label ??
    refund.payee_type

  const parsedCustomQuota = parseQuotaInput(customAmount)
  const frozenQ = refund.frozen_quota || 0

  const resolvePreviewText = useMemo(() => {
    if (quotaMode === 'subtract') {
      if (parsedCustomQuota === null || parsedCustomQuota <= 0) {
        return t('Enter a deduction amount greater than 0')
      }
      if (targetUserQuota === null) {
        return `${t('Will deduct from balance after unfreeze')}: ${formatQuota(parsedCustomQuota)}`
      }
      const after = targetUserQuota + frozenQ - parsedCustomQuota
      if (after < 0) {
        return t('Deduction exceeds available balance after unfreeze')
      }
      return `${t('Balance after unfreeze')} ${formatQuota(targetUserQuota + frozenQ)} − ${formatQuota(parsedCustomQuota)} = ${formatQuota(after)}`
    }
    if (quotaMode === 'override') {
      if (parsedCustomQuota === null) {
        return t('Enter the final balance (can be 0)')
      }
      if (targetUserQuota === null) {
        return `${t('Final balance will be set to')} ${formatQuota(parsedCustomQuota)}`
      }
      return `${t('Balance after unfreeze')} ${formatQuota(targetUserQuota + frozenQ)} → ${formatQuota(parsedCustomQuota)}`
    }
    return ''
  }, [quotaMode, parsedCustomQuota, targetUserQuota, frozenQ, t])

  const isResolveEnabled = useMemo(() => {
    if (quotaMode === 'write_off') return true
    if (parsedCustomQuota === null) return false
    if (quotaMode === 'subtract') {
      if (parsedCustomQuota <= 0) return false
      if (targetUserQuota !== null) {
        const available = targetUserQuota + frozenQ
        if (parsedCustomQuota > available) return false
      }
      return true
    }
    return parsedCustomQuota >= 0
  }, [quotaMode, parsedCustomQuota, targetUserQuota, frozenQ])

  const handleResolve = () => {
    const extra: { quota_mode: string; actual_refund_quota?: number } = {
      quota_mode: quotaMode,
    }
    if (quotaMode !== 'write_off') {
      extra.actual_refund_quota = parsedCustomQuota ?? 0
    }
    onStatusChange?.(REFUND_STATUS.REFUNDED, extra)
    setResolveOpen(false)
  }

  const handleReject = () => {
    const reason = rejectReason.trim()
    if (reason) {
      onSendMessage?.(`${t('Rejection reason')}:\n${reason}`)
    }
    onStatusChange?.(REFUND_STATUS.REJECTED)
    setRejectOpen(false)
    setRejectReason('')
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">{t('Refund Detail')}</CardTitle>
          {statusCfg && (
            <StatusBadge
              label={t(statusCfg.labelKey)}
              variant={statusCfg.variant}
              copyable={false}
            />
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-3 gap-3">
            <div className="bg-muted/50 rounded-lg p-3 text-center">
              <div className="text-muted-foreground text-xs">
                {t('Requested Amount')}
              </div>
              <div className="mt-1 font-mono text-sm font-semibold">
                {formatQuota(refund.refund_quota)}
              </div>
            </div>
            <div className="bg-muted/50 rounded-lg p-3 text-center">
              <div className="text-muted-foreground text-xs">
                {t('Frozen Amount')}
              </div>
              <div className="mt-1 font-mono text-sm font-semibold">
                {formatQuota(refund.frozen_quota)}
              </div>
            </div>
            <div className="bg-muted/50 rounded-lg p-3 text-center">
              <div className="text-muted-foreground text-xs">
                {t('Snapshot Balance')}
              </div>
              <div className="mt-1 font-mono text-sm font-semibold">
                {formatQuota(refund.user_quota_snapshot)}
              </div>
            </div>
          </div>

          <dl className="grid grid-cols-1 gap-x-4 gap-y-2 text-sm sm:grid-cols-2">
            <CopyField label={t('Payee Type')} value={t(payeeLabel)} />
            <CopyField label={t('Payee Name')} value={refund.payee_name} />
            <CopyField label={t('Payee Account')} value={refund.payee_account} />
            {refund.payee_bank && (
              <CopyField label={t('Payee Bank')} value={refund.payee_bank} />
            )}
            <CopyField label={t('Contact')} value={refund.contact} />
            <CopyField label={t('Reason')} value={refund.reason} />
            <CopyField
              label={t('Created')}
              value={formatTimestampToDate(refund.created_time)}
            />
            {refund.processed_time > 0 && (
              <CopyField
                label={t('Processed')}
                value={formatTimestampToDate(refund.processed_time)}
              />
            )}
          </dl>

          {!readonly && refund.refund_status === REFUND_STATUS.PENDING && (
            <div className="flex gap-2 pt-2">
              <Button
                size="sm"
                disabled={loading}
                onClick={() => setResolveOpen(true)}
              >
                {t('Complete Refund')}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                disabled={loading}
                onClick={() => setRejectOpen(true)}
              >
                {t('Reject')}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Resolve Dialog */}
      <Dialog open={resolveOpen} onOpenChange={setResolveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Complete Refund')}</DialogTitle>
            <DialogDescription>
              {t('Choose how to process the refund quota')}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <Alert>
              <AlertDescription>
                {t('This action will be logged')}
              </AlertDescription>
            </Alert>

            {/* Payee summary */}
            <div className="bg-muted/50 rounded-lg p-3 text-sm">
              <div className="text-muted-foreground mb-1 text-xs font-medium">
                {t('Payee Summary')}
              </div>
              <div className="grid grid-cols-2 gap-1">
                <span className="text-muted-foreground">{t('Name')}:</span>
                <span>{refund.payee_name || '-'}</span>
                <span className="text-muted-foreground">{t('Type')}:</span>
                <span>{t(payeeLabel)}</span>
                <span className="text-muted-foreground">{t('Account')}:</span>
                <span>{refund.payee_account || '-'}</span>
                {refund.payee_bank && (
                  <>
                    <span className="text-muted-foreground">{t('Bank')}:</span>
                    <span>{refund.payee_bank}</span>
                  </>
                )}
              </div>
            </div>

            {targetUserQuota !== null && (
              <div className="text-muted-foreground text-sm">
                {t('Current user balance')}: {formatQuota(targetUserQuota)}
              </div>
            )}

            <div>
              <Label>{t('Quota Mode')}</Label>
              <Select value={quotaMode} onValueChange={setQuotaMode}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="write_off">
                    {t('Write Off (Recommended)')}
                  </SelectItem>
                  <SelectItem value="subtract">{t('Subtract')}</SelectItem>
                  <SelectItem value="override">{t('Override')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {quotaMode !== 'write_off' && (
              <div>
                <Label>
                  {quotaMode === 'subtract'
                    ? t('Deduction Amount')
                    : t('Target Balance')}
                </Label>
                <Input
                  type="number"
                  value={customAmount}
                  onChange={(e) => setCustomAmount(e.target.value)}
                  min={0}
                />
              </div>
            )}
            {resolvePreviewText && (
              <div className="text-muted-foreground text-sm">
                {resolvePreviewText}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResolveOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleResolve} disabled={loading || !isResolveEnabled}>
              {t('Confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reject Dialog */}
      <Dialog open={rejectOpen} onOpenChange={setRejectOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Reject Refund')}</DialogTitle>
            <DialogDescription>
              {t('Optionally provide a reason for rejection')}
            </DialogDescription>
          </DialogHeader>
          <Alert>
            <AlertDescription>
              {t('Rejection will unfreeze the quota. The user can reapply.')}
            </AlertDescription>
          </Alert>
          <Textarea
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            placeholder={t('Rejection reason (optional)')}
            maxLength={500}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRejectOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleReject}
              disabled={loading}
            >
              {t('Reject')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
