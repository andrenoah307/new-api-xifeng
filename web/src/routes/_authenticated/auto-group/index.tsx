import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { AutoGroup } from '@/features/auto-group'

const autoGroupSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  filter: z.string().optional().catch(''),
  ePage: z.number().optional().catch(1),
  ePageSize: z.number().optional().catch(undefined),
  eFilter: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/auto-group/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: autoGroupSearchSchema,
  component: AutoGroup,
})
