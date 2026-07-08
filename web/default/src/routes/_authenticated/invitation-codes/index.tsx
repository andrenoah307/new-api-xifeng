import { createFileRoute } from '@tanstack/react-router'
import InvitationCodesPage from '@/features/invitation-codes'

export const Route = createFileRoute('/_authenticated/invitation-codes/')({
  component: InvitationCodesPage,
})
