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
  return <TicketAdminDetailPage ticketId={Number(ticketId)} />
}
