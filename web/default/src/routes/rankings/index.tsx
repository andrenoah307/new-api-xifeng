import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { Rankings } from '@/features/rankings'

const rankingsSearchSchema = z.object({
  period: z
    .enum(['today', 'week', 'month', 'year', 'all'])
    .optional()
    .catch(undefined),
})

export const Route = createFileRoute('/rankings/')({
  beforeLoad: ({ location }) => {
    const { auth } = useAuthStore.getState()
    if (!auth.user) {
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }
    if (auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: rankingsSearchSchema,
  component: Rankings,
})
