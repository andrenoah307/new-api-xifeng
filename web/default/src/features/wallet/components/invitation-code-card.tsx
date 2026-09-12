/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { KeyRound, Link as LinkIcon, Loader2, RefreshCw, UserPlus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  generateUserInvitationCode,
  getUserInvitationCodes,
  getUserInvitationQuota,
  type InvitationCode,
  type InvitationQuotaInfo,
} from '@/features/invitation-codes/api'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatTimestamp } from '@/lib/format'

// 邀请链接格式与 Classic 保持一致（web/classic/src/components/invitation/InvitationCodeCard.jsx）
function buildInviteLink(code: string) {
  return `${window.location.origin}/register?code=${code}`
}

export function InvitationCodeCard() {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [quotaInfo, setQuotaInfo] = useState<InvitationQuotaInfo | null>(null)
  const [codes, setCodes] = useState<InvitationCode[]>([])

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [quota, list] = await Promise.all([
        getUserInvitationQuota(),
        getUserInvitationCodes(),
      ])
      setQuotaInfo(quota)
      setCodes(list)
    } catch (_error) {
      // Errors are surfaced by the global interceptor
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  const handleGenerate = async () => {
    setGenerating(true)
    try {
      const code = await generateUserInvitationCode()
      if (code) {
        toast.success(t('Invitation code created'))
        // 与 Classic 一致：生成成功后直接把邀请链接放进剪贴板
        await copyToClipboard(buildInviteLink(code))
      }
      await loadData()
    } catch (_error) {
      // Errors are surfaced by the global interceptor
    } finally {
      setGenerating(false)
    }
  }

  const stats = useMemo(() => {
    if (!quotaInfo) {
      return [
        [t('Remaining generations'), '-'],
        [t('Default max uses'), '-'],
        [t('Default validity'), '-'],
      ] as const
    }
    // limit < 0 表示不限次数；max_uses / valid_days 为 0 表示不限次数 / 永久，
    // 三条语义与 Classic 的 InvitationCodeCard.jsx 完全一致。
    const remaining =
      quotaInfo.limit < 0 ? t('Unlimited') : String(quotaInfo.remaining)
    const maxUses =
      quotaInfo.default_code_max_uses === 0
        ? t('Unlimited')
        : String(quotaInfo.default_code_max_uses)
    const validDays =
      quotaInfo.default_code_valid_days === 0
        ? t('Never expires')
        : t('{{days}} days', { days: quotaInfo.default_code_valid_days })
    return [
      [t('Remaining generations'), remaining],
      [t('Default max uses'), maxUses],
      [t('Default validity'), validDays],
    ] as const
  }, [quotaInfo, t])

  if (loading) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='grid gap-4 p-3 sm:p-4'>
          <Skeleton className='h-5 w-40' />
          <Skeleton className='h-14 rounded-lg' />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card data-card-hover='false' className='bg-muted/20 py-0'>
      <CardContent className='flex flex-col gap-3 p-3 sm:gap-4 sm:p-4'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex min-w-0 items-center gap-2.5'>
            <IconBadge tone='chart-1'>
              <UserPlus />
            </IconBadge>
            <div className='min-w-0'>
              <h3 className='truncate text-sm font-semibold'>
                {t('Invitation Codes')}
              </h3>
              <p className='text-muted-foreground line-clamp-1 text-xs'>
                {t(
                  'Generate your own registration link. The number of invitations is controlled by the administrator policy.'
                )}
              </p>
            </div>
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              className='h-9 gap-1.5'
              onClick={loadData}
            >
              <RefreshCw className='size-4' />
              {t('Refresh')}
            </Button>
            <Button
              size='sm'
              className='h-9 gap-1.5'
              disabled={generating || !quotaInfo?.can_generate}
              onClick={handleGenerate}
            >
              {generating ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <KeyRound className='size-4' />
              )}
              {t('Generate invitation code')}
            </Button>
          </div>
        </div>

        <div className='grid grid-cols-3 gap-1.5 text-center'>
          {stats.map(([label, value]) => (
            <div key={label}>
              <div className='text-muted-foreground truncate text-[10px] font-medium tracking-wider uppercase'>
                {label}
              </div>
              <div className='mt-0.5 truncate text-sm font-semibold tabular-nums'>
                {value}
              </div>
            </div>
          ))}
        </div>

        {quotaInfo?.reason ? (
          <p className='text-muted-foreground text-xs'>{quotaInfo.reason}</p>
        ) : null}

        {codes.length === 0 ? (
          <p className='text-muted-foreground py-2 text-center text-xs'>
            {t('No invitation codes yet')}
          </p>
        ) : (
          <div className='flex flex-col gap-2'>
            {codes.map((record) => (
              <div
                key={record.id}
                className='border-muted bg-background/70 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border p-2.5'
              >
                <span className='font-mono text-sm font-semibold'>
                  {record.code}
                </span>
                <InvitationCodeStatus record={record} />
                <span className='text-muted-foreground text-xs'>
                  {t('Usage')}: {record.used_count}/
                  {record.max_uses === 0 ? t('Unlimited') : record.max_uses}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {t('Expires')}:{' '}
                  {record.expired_time === 0
                    ? t('Never expires')
                    : formatTimestamp(record.expired_time)}
                </span>
                <div className='ms-auto flex items-center gap-1'>
                  <CopyButton
                    value={record.code}
                    variant='outline'
                    className='bg-background size-8'
                    iconClassName='size-3.5'
                    tooltip={t('Copy invitation code')}
                    aria-label={t('Copy invitation code')}
                  />
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant='outline'
                          size='icon'
                          className='bg-background size-8'
                          aria-label={t('Copy invitation link')}
                          onClick={() =>
                            copyToClipboard(buildInviteLink(record.code))
                          }
                        >
                          <LinkIcon className='size-3.5' />
                        </Button>
                      }
                    />
                    <TooltipContent>
                      <p>{t('Copy invitation link')}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function InvitationCodeStatus({ record }: { record: InvitationCode }) {
  const { t } = useTranslation()

  if (record.status !== 1) {
    return <StatusBadge label={t('Disabled')} variant='danger' copyable={false} />
  }
  if (record.expired_time !== 0 && record.expired_time < Math.floor(Date.now() / 1000)) {
    return <StatusBadge label={t('Expired')} variant='warning' copyable={false} />
  }
  if (record.max_uses > 0 && record.used_count >= record.max_uses) {
    return <StatusBadge label={t('Used up')} variant='purple' copyable={false} />
  }
  return <StatusBadge label={t('Available')} variant='success' copyable={false} />
}
