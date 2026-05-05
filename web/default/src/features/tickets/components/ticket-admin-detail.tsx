import { useState, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { formatTimestampToDate } from '@/lib/format'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SectionPageLayout } from '@/components/layout'
import {
  getAdminTicketDetail,
  getAdminInvoiceDetail,
  getAdminRefundDetail,
  sendAdminMessage,
  updateTicketStatus,
  assignTicket,
  updateRefundStatus,
  updateInvoiceStatus,
} from '../api'
import {
  canReply,
  getStatusOptions,
  getPriorityOptions,
} from '../constants'
import { ticketQueryKeys } from '../lib/ticket-actions'
import {
  TicketStatusBadge,
  TicketPriorityBadge,
  TicketTypeBadge,
} from './ticket-status-badge'
import { TicketConversation } from './ticket-conversation'
import { TicketReplyBox } from './ticket-reply-box'
import { InvoiceDetail } from './invoice-detail'
import { RefundDetail } from './refund-detail'
import { TicketUserProfileButton } from './ticket-user-profile'

export default function TicketAdminDetailPage({
  ticketId,
}: {
  ticketId: number
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const account = useAuthStore((s) => s.auth.user)
  const accountId = account?.id ?? 0

  const [statusValue, setStatusValue] = useState<string>('')
  const [priorityValue, setPriorityValue] = useState<string>('')

  const { data, isLoading } = useQuery({
    queryKey: ticketQueryKeys.adminDetail(ticketId),
    queryFn: () => getAdminTicketDetail(ticketId),
  })

  const ticket = data?.ticket
  const messages = data?.messages ?? []

  // Sync local state when ticket loads
  if (ticket && !statusValue) {
    setStatusValue(String(ticket.status))
    setPriorityValue(String(ticket.priority))
  }

  const isInvoice = ticket?.type === 'invoice'
  const isRefund = ticket?.type === 'refund'

  const conversationMessages = useMemo(() => {
    if (messages.length === 0) return messages
    const first = messages[0]
    if (isRefund && first?.content?.startsWith('退款申请信息')) {
      return messages.slice(1)
    }
    if (isInvoice && first?.content?.startsWith('发票申请信息')) {
      return messages.slice(1)
    }
    return messages
  }, [messages, isRefund, isInvoice])

  const { data: invoiceData } = useQuery({
    queryKey: ticketQueryKeys.adminInvoice(ticketId),
    queryFn: () => getAdminInvoiceDetail(ticketId),
    enabled: isInvoice && !!ticket,
  })

  const { data: refundData } = useQuery({
    queryKey: ticketQueryKeys.adminRefund(ticketId),
    queryFn: () => getAdminRefundDetail(ticketId),
    enabled: isRefund && !!ticket,
  })

  const replyMutation = useMutation({
    mutationFn: ({
      content,
      attachmentIds,
    }: {
      content: string
      attachmentIds: number[]
    }) => sendAdminMessage(ticketId, content, attachmentIds),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminDetail(ticketId),
      })
    },
  })

  const statusMutation = useMutation({
    mutationFn: () =>
      updateTicketStatus(
        ticketId,
        Number(statusValue),
        Number(priorityValue)
      ),
    onSuccess: () => {
      toast.success(t('Status updated'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminDetail(ticketId),
      })
    },
  })

  const claimMutation = useMutation({
    mutationFn: () => assignTicket(ticketId, accountId, 0),
    onSuccess: () => {
      toast.success(t('Ticket claimed'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminDetail(ticketId),
      })
    },
  })

  const refundStatusMutation = useMutation({
    mutationFn: ({
      status,
      extra,
    }: {
      status: number
      extra?: { quota_mode?: string; actual_refund_quota?: number }
    }) => updateRefundStatus(ticketId, status, extra),
    onSuccess: () => {
      toast.success(t('Operation successful'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminRefund(ticketId),
      })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminDetail(ticketId),
      })
    },
  })

  const invoiceStatusMutation = useMutation({
    mutationFn: (status: number) => updateInvoiceStatus(ticketId, status),
    onSuccess: () => {
      toast.success(t('Operation successful'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminInvoice(ticketId),
      })
    },
  })

  const handleReply = useCallback(
    async (content: string, attachmentIds: number[]) => {
      await replyMutation.mutateAsync({ content, attachmentIds })
    },
    [replyMutation]
  )

  const handleRefundStatusChange = useCallback(
    (
      status: number,
      extra?: { quota_mode?: string; actual_refund_quota?: number }
    ) => {
      refundStatusMutation.mutate({ status, extra })
    },
    [refundStatusMutation]
  )

  const handleSendSystemMessage = useCallback(
    (content: string) => {
      replyMutation.mutate({ content, attachmentIds: [] })
    },
    [replyMutation]
  )

  if (isLoading) {
    return (
      <div className="space-y-4 p-6">
        <Skeleton className="h-8 w-1/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!ticket) {
    return (
      <div className="py-24 text-center">
        <p className="text-muted-foreground">{t('Ticket not found')}</p>
      </div>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate({ to: '/ticket-admin' })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <span className="truncate">{ticket.subject}</span>
          <TicketStatusBadge status={ticket.status} />
          <TicketTypeBadge type={ticket.type} />
          {ticket.assignee_id === 0 ? (
            <StatusBadge label={t('Unassigned')} variant="neutral" copyable={false} />
          ) : ticket.assignee_id === accountId ? (
            <StatusBadge label={t('Assigned to me')} variant="success" copyable={false} />
          ) : (
            <StatusBadge label={`${t('Processing')} · #${ticket.assignee_id}`} variant="info" copyable={false} />
          )}
        </div>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className="flex flex-wrap items-center gap-2">
          {ticket.assignee_id === 0 && (
            <Button
              size="sm"
              onClick={() => claimMutation.mutate()}
              disabled={claimMutation.isPending}
            >
              {t('Claim Ticket')}
            </Button>
          )}
          <Select value={statusValue} onValueChange={setStatusValue}>
            <SelectTrigger className="h-8 w-[120px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {getStatusOptions(true).map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {t(o.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={priorityValue} onValueChange={setPriorityValue}>
            <SelectTrigger className="h-8 w-[120px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {getPriorityOptions().map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {t(o.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            size="sm"
            variant="outline"
            onClick={() => statusMutation.mutate()}
            disabled={statusMutation.isPending}
          >
            {t('Save')}
          </Button>
          {ticket?.user_id && (
            <TicketUserProfileButton ticketId={ticketId} />
          )}
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className="space-y-6">
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">ID</dt>
              <dd className="font-mono">#{ticket.id}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('Type')}</dt>
              <dd><TicketTypeBadge type={ticket.type} /></dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('Priority')}</dt>
              <dd><TicketPriorityBadge priority={ticket.priority} /></dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('User')}</dt>
              <dd>{ticket.username || '-'} <span className="text-muted-foreground">(#{ticket.user_id})</span></dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('Created')}</dt>
              <dd>{formatTimestampToDate(ticket.created_time)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('Updated')}</dt>
              <dd>{formatTimestampToDate(ticket.updated_time)}</dd>
            </div>
          </dl>

          {isInvoice && invoiceData?.invoice && (
            <InvoiceDetail
              invoice={invoiceData.invoice}
              orders={invoiceData.orders ?? []}
              onStatusChange={(s) => invoiceStatusMutation.mutate(s)}
              loading={invoiceStatusMutation.isPending}
            />
          )}
          {isRefund && refundData && (
            <RefundDetail
              refund={refundData}
              onStatusChange={handleRefundStatusChange}
              onSendMessage={handleSendSystemMessage}
              loading={refundStatusMutation.isPending}
            />
          )}

          <Separator />

          <TicketConversation
            messages={conversationMessages}
            currentUserId={accountId}
          />

          {canReply(ticket.status) && (
            <TicketReplyBox
              onSubmit={handleReply}
              loading={replyMutation.isPending}
              placeholder={t('Admin reply...')}
            />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
