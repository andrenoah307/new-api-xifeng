import z from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import TopupHistoryPage from '@/features/topup-history'

const topupHistorySearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(20),
  keyword: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/topup-history/')({
  validateSearch: topupHistorySearchSchema,
  component: TopupHistoryPage,
})
