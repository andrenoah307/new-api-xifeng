import z from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import TicketListPage from '@/features/tickets/components/ticket-list'

const ticketsSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  status: z.array(z.enum(['1', '2', '3'])).optional().catch([]),
  type: z
    .array(z.enum(['general', 'refund', 'invoice']))
    .optional()
    .catch([]),
})

export const Route = createFileRoute('/_authenticated/tickets/')({
  validateSearch: ticketsSearchSchema,
  component: TicketListPage,
})
