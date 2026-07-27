import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import InvitationCodesPage from '@/features/invitation-codes'

const invitationCodesSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  keyword: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/invitation-codes/')({
  // 后端接口是 AdminAuth，前端路由同样只放行管理员（与 auto-group 一致）
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: invitationCodesSearchSchema,
  component: InvitationCodesPage,
})
