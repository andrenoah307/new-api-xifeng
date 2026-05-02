import { createFileRoute } from '@tanstack/react-router'
import MonitoringDashboard from '@/features/monitoring'

export const Route = createFileRoute('/_authenticated/monitoring/')({
  component: MonitoringDashboard,
})
