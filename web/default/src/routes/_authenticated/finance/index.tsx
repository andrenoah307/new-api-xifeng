import { createFileRoute, redirect } from '@tanstack/react-router'

import FinancePage from '@/features/finance'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/finance/')({
  beforeLoad: () => {
    if (!hasPermission(useAuthStore.getState().auth.user, 'finance', 'read')) {
      throw redirect({ to: '/403' })
    }
  },
  component: FinancePage,
})
