import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import TicketAdminDetailPage from '@/features/tickets/components/ticket-admin-detail'

export const Route = createFileRoute('/_authenticated/ticket-admin/$ticketId')({
  beforeLoad: ({ params }) => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.TICKET_STAFF) {
      throw redirect({ to: '/403' })
    }
    if (!Number.isInteger(Number(params.ticketId))) {
      throw redirect({ to: '/ticket-admin' })
    }
  },
  component: TicketAdminDetailRoute,
})

function TicketAdminDetailRoute() {
  const { ticketId } = Route.useParams()
  // key 保证跨工单切换时组件重挂载，本地状态（状态/优先级下拉）不会串号
  return <TicketAdminDetailPage key={ticketId} ticketId={Number(ticketId)} />
}
