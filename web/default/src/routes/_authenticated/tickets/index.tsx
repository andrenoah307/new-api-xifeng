import z from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import TicketListPage from '@/features/tickets/components/ticket-list'

const ticketsSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
})

export const Route = createFileRoute('/_authenticated/tickets/')({
  validateSearch: ticketsSearchSchema,
  component: TicketListPage,
})
