import z from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import TopupHistoryPage from '@/features/topup-history'

const topupHistorySearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(20),
  keyword: z.string().optional().catch(''),
  status: z
    .array(z.enum(['success', 'pending', 'failed', 'expired']))
    .optional()
    .catch([]),
  start: z.number().optional().catch(undefined),
  end: z.number().optional().catch(undefined),
  // 创建工单选"开票工单"跳转过来时自动打开申请开票对话框
  applyInvoice: z.boolean().optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/topup-history/')({
  validateSearch: topupHistorySearchSchema,
  component: TopupHistoryPage,
})
