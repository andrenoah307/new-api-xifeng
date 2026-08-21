import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import TicketAdminListPage from '@/features/tickets/components/ticket-admin-list'
import { ticketAdminSearchSchema } from '@/features/tickets/lib/ticket-admin-search'

export const Route = createFileRoute('/_authenticated/ticket-admin/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.TICKET_STAFF) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: ticketAdminSearchSchema,
  component: TicketAdminListPage,
})
