import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { User, Mail, Shield, Clock, Activity, Database, RefreshCw } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { StatusBadge } from '@/components/status-badge'
import { formatQuota } from '@/lib/format'
import { formatTimestampToDate } from '@/lib/format'
import { getRoleLabelKey } from '@/lib/roles'
import { getAdminUserProfile } from '../api'
import { ticketQueryKeys } from '../lib/ticket-actions'

interface TicketUserProfileButtonProps {
  ticketId: number
}

export function TicketUserProfileButton({
  ticketId,
}: TicketUserProfileButtonProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const { data: profile, isLoading } = useQuery({
    queryKey: ticketQueryKeys.adminUserProfile(ticketId),
    queryFn: () => getAdminUserProfile(ticketId),
    enabled: open,
  })

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <User className="h-3.5 w-3.5" />
          {t('User Profile')}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] max-w-3xl overflow-y-auto">
        <DialogHeader className="flex flex-row items-center justify-between pr-8">
          <DialogTitle className="flex items-center gap-2">
            <User className="h-4 w-4" />
            {t('User Profile')}
          </DialogTitle>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={() =>
              queryClient.invalidateQueries({
                queryKey: ticketQueryKeys.adminUserProfile(ticketId),
              })
            }
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </DialogHeader>

        {isLoading ? (
          <div className="space-y-3 py-4">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        ) : !profile ? (
          <p className="text-muted-foreground py-8 text-center text-sm">
            {t('No data available')}
          </p>
        ) : (
          <ProfileContent profile={profile} t={t} />
        )}
      </DialogContent>
    </Dialog>
  )
}

function ProfileContent({
  profile,
  t,
}: {
  profile: NonNullable<Awaited<ReturnType<typeof getAdminUserProfile>>>
  t: (key: string) => string
}) {
  const statusVariant = profile.status === 1 ? 'success' : 'neutral'
  const statusLabel = profile.status === 1 ? t('Enabled') : t('Disabled')

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-3">
        <InfoItem label={t('Username')} value={profile.username} />
        <InfoItem label={t('UID')} value={String(profile.user_id)} mono />
        <InfoItem
          label={t('Display Name')}
          value={profile.display_name || '-'}
        />
        <InfoItem
          label={t('Email')}
          value={profile.email || '-'}
          icon={<Mail className="h-3 w-3" />}
        />
        <InfoItem
          label={t('Role')}
          value={t(getRoleLabelKey(profile.role))}
          icon={<Shield className="h-3 w-3" />}
        />
        <div>
          <dt className="text-muted-foreground text-xs">{t('Status')}</dt>
          <dd className="mt-0.5">
            <StatusBadge
              label={statusLabel}
              variant={statusVariant}
              size="sm"
              copyable={false}
            />
          </dd>
        </div>
        <InfoItem label={t('Group')} value={profile.group || '-'} />
        <InfoItem
          label={t('Registered')}
          value={formatTimestampToDate(profile.created_time)}
          icon={<Clock className="h-3 w-3" />}
        />
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label={t('Balance')} value={formatQuota(profile.quota)} />
        <StatCard
          label={t('Used Quota')}
          value={formatQuota(profile.used_quota)}
        />
        <StatCard
          label={t('Requests')}
          value={profile.request_count.toLocaleString()}
        />
        <StatCard
          label={t('Pending Refund')}
          value={formatQuota(profile.pending_refund_quota)}
        />
      </div>

      {profile.recent_logs && profile.recent_logs.length > 0 && (
        <div>
          <h4 className="text-muted-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
            <Activity className="h-3 w-3" />
            {t('Recent API Calls')}
          </h4>
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-xs">
              <thead className="bg-muted/30">
                <tr>
                  <th className="px-2 py-1.5 text-left font-medium">
                    {t('Time')}
                  </th>
                  <th className="px-2 py-1.5 text-left font-medium">
                    {t('Model')}
                  </th>
                  <th className="px-2 py-1.5 text-left font-medium">
                    {t('Token')}
                  </th>
                  <th className="px-2 py-1.5 text-left font-medium">
                    {t('Group')}
                  </th>
                  <th className="px-2 py-1.5 text-right font-medium">
                    {t('Quota')}
                  </th>
                  <th className="px-2 py-1.5 text-right font-medium">
                    {t('Tokens')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {profile.recent_logs.map((log, i) => (
                  <tr key={i} className="border-t">
                    <td className="text-muted-foreground px-2 py-1.5 font-mono">
                      {formatTimestampToDate(log.created_at)}
                    </td>
                    <td className="max-w-[120px] truncate px-2 py-1.5">
                      {log.model_name}
                    </td>
                    <td className="max-w-[100px] truncate px-2 py-1.5">
                      {log.token_name}
                    </td>
                    <td className="max-w-[80px] truncate px-2 py-1.5">
                      {log.group || '-'}
                    </td>
                    <td className="px-2 py-1.5 text-right font-mono">
                      {formatQuota(log.quota)}
                    </td>
                    <td className="text-muted-foreground px-2 py-1.5 text-right font-mono">
                      {(
                        (log.prompt_tokens || 0) +
                        (log.completion_tokens || 0)
                      ).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {profile.model_usage && profile.model_usage.length > 0 && (
        <div>
          <h4 className="text-muted-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
            <Database className="h-3 w-3" />
            {t('Model Usage (30 days)')}
          </h4>
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-xs">
              <thead className="bg-muted/30">
                <tr>
                  <th className="px-2 py-1.5 text-left font-medium">
                    {t('Model')}
                  </th>
                  <th className="px-2 py-1.5 text-right font-medium">
                    {t('Calls')}
                  </th>
                  <th className="px-2 py-1.5 text-right font-medium">
                    {t('Quota')}
                  </th>
                  <th className="px-2 py-1.5 text-right font-medium">
                    {t('Tokens')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {profile.model_usage.map((usage, i) => (
                  <tr key={i} className="border-t">
                    <td className="max-w-[160px] truncate px-2 py-1.5 font-medium">
                      {usage.model_name}
                    </td>
                    <td className="px-2 py-1.5 text-right font-mono">
                      {usage.count.toLocaleString()}
                    </td>
                    <td className="px-2 py-1.5 text-right font-mono">
                      {formatQuota(usage.quota)}
                    </td>
                    <td className="text-muted-foreground px-2 py-1.5 text-right font-mono">
                      {(usage.token_used || 0).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function InfoItem({
  label,
  value,
  icon,
  mono,
}: {
  label: string
  value: string
  icon?: React.ReactNode
  mono?: boolean
}) {
  return (
    <div>
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd
        className={`mt-0.5 flex items-center gap-1 text-sm ${mono ? 'font-mono' : ''}`}
      >
        {icon && <span className="text-muted-foreground/70">{icon}</span>}
        {value}
      </dd>
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-muted/50 rounded-lg p-3 text-center">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="mt-1 font-mono text-sm font-semibold">{value}</div>
    </div>
  )
}
