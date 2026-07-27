import { createFileRoute, redirect } from '@tanstack/react-router'
import TicketDetailPage from '@/features/tickets/components/ticket-detail'

export const Route = createFileRoute('/_authenticated/tickets/$ticketId')({
  beforeLoad: ({ params }) => {
    if (!Number.isInteger(Number(params.ticketId))) {
      throw redirect({ to: '/tickets' })
    }
  },
  component: TicketDetailRoute,
})

function TicketDetailRoute() {
  const { ticketId } = Route.useParams()
  return <TicketDetailPage ticketId={Number(ticketId)} />
}
