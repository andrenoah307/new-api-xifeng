import z from 'zod'

export const ticketAdminSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  keyword: z.string().optional().catch(''),
  scope: z.enum(['all', 'mine', 'unassigned']).optional().catch(undefined),
  status: z.string().optional().catch(''),
  type: z.string().optional().catch(''),
})

export type TicketAdminSearch = z.infer<typeof ticketAdminSearchSchema>
