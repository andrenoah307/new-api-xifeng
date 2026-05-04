import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { SectionPageLayout } from '@/components/layout'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { useState } from 'react'
import {
  getUserTicketDetail,
  sendUserMessage,
  closeUserTicket,
} from '../api'
import { canClose, canReply } from '../constants'
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

export default function TicketDetailPage({
  ticketId,
}: {
  ticketId: number
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const userId = useAuthStore((s) => s.auth.user?.id ?? 0)
  const [closeOpen, setCloseOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ticketQueryKeys.userDetail(ticketId),
    queryFn: () => getUserTicketDetail(ticketId),
  })

  const ticket = data?.ticket
  const messages = data?.messages ?? []
  const invoice = data?.invoice
  const invoiceOrders = data?.invoice_orders ?? []
  const refund = data?.refund

  const conversationMessages = useMemo(() => {
    if (!refund || messages.length === 0) return messages
    const first = messages[0]
    if (first && first.content?.startsWith('退款申请信息')) {
      return messages.slice(1)
    }
    return messages
  }, [messages, refund])

  const replyMutation = useMutation({
    mutationFn: ({
      content,
      attachmentIds,
    }: {
      content: string
      attachmentIds: number[]
    }) => sendUserMessage(ticketId, content, attachmentIds),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userDetail(ticketId),
      })
    },
  })

  const closeMutation = useMutation({
    mutationFn: () => closeUserTicket(ticketId),
    onSuccess: () => {
      toast.success(t('Ticket closed'))
      setCloseOpen(false)
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userDetail(ticketId),
      })
    },
  })

  const handleReply = useCallback(
    async (content: string, attachmentIds: number[]) => {
      await replyMutation.mutateAsync({ content, attachmentIds })
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
    <>
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate({ to: '/tickets' })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <span className="truncate">{ticket.subject}</span>
        </div>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className="flex items-center gap-2">
          <TicketStatusBadge status={ticket.status} />
          <TicketPriorityBadge priority={ticket.priority} />
          <TicketTypeBadge type={ticket.type} />
          {canClose(ticket.status) && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setCloseOpen(true)}
            >
              {t('Close Ticket')}
            </Button>
          )}
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className="space-y-6">
          {invoice && (
            <InvoiceDetail
              invoice={invoice}
              orders={invoiceOrders}
              readonly
            />
          )}
          {refund && <RefundDetail refund={refund} readonly />}

          <Separator />

          <TicketConversation
            messages={conversationMessages}
            currentUserId={userId}
          />

          {canReply(ticket.status) && (
            <TicketReplyBox
              onSubmit={handleReply}
              loading={replyMutation.isPending}
            />
          )}
        </div>
      </SectionPageLayout.Content>

    </SectionPageLayout>
    <ConfirmDialog
      open={closeOpen}
      onOpenChange={setCloseOpen}
      title={t('Close Ticket')}
      desc={t('Are you sure you want to close this ticket?')}
      handleConfirm={() => closeMutation.mutate()}
      isLoading={closeMutation.isPending}
    />
    </>
  )
}
