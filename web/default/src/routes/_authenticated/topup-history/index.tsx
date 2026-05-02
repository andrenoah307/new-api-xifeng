import { createFileRoute } from '@tanstack/react-router'
import TopupHistoryPage from '@/features/topup-history'

export const Route = createFileRoute('/_authenticated/topup-history/')({
  component: TopupHistoryPage,
})
