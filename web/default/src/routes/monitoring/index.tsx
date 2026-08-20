/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { AuthenticatedLayout, Main, PublicLayout } from '@/components/layout'
import MonitoringDashboard from '@/features/monitoring'
import { getStatus } from '@/lib/api'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/monitoring/')({
  // 分组监控无需登录即可查看，仅受「顶部导航-分组监控」开关控制。
  beforeLoad: async () => {
    const status = await getStatus().catch(() => null)
    const modules = parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
    if (modules.monitoring === false) {
      throw redirect({ to: '/' })
    }
  },
  component: MonitoringPage,
})

function MonitoringPage() {
  // 登录用户保留控制台外壳（侧边栏），未登录访客用公开布局。
  // 同一路径不能挂两个路由，AuthenticatedLayout 支持 children 直接嵌入。
  const user = useAuthStore((state) => state.auth.user)

  if (user) {
    return (
      <AuthenticatedLayout>
        <Main className='p-0'>
          {/* AuthenticatedLayout 是固定高裁剪外壳，登录页面必须自带滚动容器；未登录分支由 PublicLayout 走文档流滚动。 */}
          <div className='min-h-0 flex-1 overflow-y-auto'>
            <MonitoringDashboard />
          </div>
        </Main>
      </AuthenticatedLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <div className='pt-16 sm:pt-20'>
        <MonitoringDashboard />
      </div>
    </PublicLayout>
  )
}
