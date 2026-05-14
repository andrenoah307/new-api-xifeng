import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { DiscountCodes } from '@/features/discount-codes'
import { DISCOUNT_CODE_STATUS_VALUES } from '@/features/discount-codes/constants'

const discountCodesSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  filter: z.string().optional().catch(''),
  status: z.array(z.enum(DISCOUNT_CODE_STATUS_VALUES)).optional().catch([]),
})

export const Route = createFileRoute('/_authenticated/discount-codes/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: discountCodesSearchSchema,
  component: DiscountCodes,
})
