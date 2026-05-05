import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import TicketAdminListPage from '@/features/tickets/components/ticket-admin-list'

const ticketAdminSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  keyword: z.string().optional().catch(''),
  scope: z.enum(['all', 'mine', 'unassigned']).optional().catch(undefined),
  status: z.string().optional().catch(''),
  type: z.string().optional().catch(''),
})

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
