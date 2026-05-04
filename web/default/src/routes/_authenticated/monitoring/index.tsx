import { createFileRoute } from '@tanstack/react-router'
import { AppHeader, Main } from '@/components/layout'
import MonitoringDashboard from '@/features/monitoring'

export const Route = createFileRoute('/_authenticated/monitoring/')({
  component: MonitoringPage,
})

function MonitoringPage() {
  return (
    <>
      <AppHeader />
      <Main className="p-0">
        <MonitoringDashboard />
      </Main>
    </>
  )
}
