import { useState, useCallback, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
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
  extractRemarkFromSummary,
  isGeneratedTicketSummary,
} from '../lib/ticket-summary'
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

const route = getRouteApi('/_authenticated/ticket-admin/$ticketId')

export default function TicketAdminDetailPage({
  ticketId,
}: {
  ticketId: number
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const search = route.useSearch()
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
  const messages = useMemo(() => data?.messages ?? [], [data?.messages])

  // Sync local state when ticket loads
  useEffect(() => {
    if (!ticket) return
    setStatusValue(String(ticket.status))
    setPriorityValue(String(ticket.priority))
  }, [ticket])

  const isInvoice = ticket?.type === 'invoice'
  const isRefund = ticket?.type === 'refund'

  const conversationMessages = useMemo(() => {
    if (messages.length === 0) return messages
    const first = messages[0]
    if (isGeneratedTicketSummary(first?.content, ticket?.type ?? '')) {
      return messages.slice(1)
    }
    return messages
  }, [messages, ticket?.type])

  const summaryRemark = useMemo(
    () => extractRemarkFromSummary(messages[0]?.content),
    [messages]
  )

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
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(),
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
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(),
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
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(),
      })
    },
  })

  const refundStatusMutation = useMutation({
    mutationFn: ({
      status,
      extra,
    }: {
      status: number
      extra?: {
        quota_mode?: string
        actual_refund_quota?: number
        claw_back_commission?: boolean
        claw_back_quota?: number
      }
    }) => updateRefundStatus(ticketId, status, extra),
    onSuccess: () => {
      toast.success(t('Operation successful'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminRefund(ticketId),
      })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminDetail(ticketId),
      })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(),
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
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(),
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
      extra?: {
        quota_mode?: string
        actual_refund_quota?: number
        claw_back_commission?: boolean
        claw_back_quota?: number
      }
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

  let assigneeBadge = (
    <StatusBadge
      label={`${t('Processing')} · #${ticket.assignee_id}`}
      variant="info"
      copyable={false}
    />
  )
  if (ticket.assignee_id === 0) {
    assigneeBadge = (
      <StatusBadge
        label={t('Unassigned')}
        variant="neutral"
        copyable={false}
      />
    )
  } else if (ticket.assignee_id === accountId) {
    assigneeBadge = (
      <StatusBadge
        label={t('Assigned to me')}
        variant="success"
        copyable={false}
      />
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate({ to: '/ticket-admin', search })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <span className="truncate">{ticket.subject}</span>
          <TicketStatusBadge status={ticket.status} />
          <TicketTypeBadge type={ticket.type} />
          {assigneeBadge}
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
          <Select
            value={statusValue}
            onValueChange={(value) => {
              if (value != null) setStatusValue(value)
            }}
          >
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
          <Select
            value={priorityValue}
            onValueChange={(value) => {
              if (value != null) setPriorityValue(value)
            }}
          >
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
              fallbackRemark={summaryRemark}
              onStatusChange={(s) => invoiceStatusMutation.mutate(s)}
              loading={invoiceStatusMutation.isPending}
            />
          )}
          {isRefund && refundData && (
            <RefundDetail
              refund={refundData.refund}
              userInvoices={refundData.user_invoices}
              commissionInfo={refundData.commission_info}
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
