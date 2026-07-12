import { createFileRoute } from '@tanstack/react-router'
import { Main } from '@/components/layout'
import MonitoringDashboard from '@/features/monitoring'

export const Route = createFileRoute('/_authenticated/monitoring/')({
  component: MonitoringPage,
})

function MonitoringPage() {
  return (
    <Main className="p-0">
      <MonitoringDashboard />
    </Main>
  )
}
