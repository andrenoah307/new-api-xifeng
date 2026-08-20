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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { MonitoringGroupWithHistory } from '../api'

interface MockContainerProps {
  children?: ReactNode
  className?: string
}

function MockSheet(props: MockContainerProps) {
  return <div data-slot='sheet'>{props.children}</div>
}

function MockContainer(props: MockContainerProps) {
  return <div className={props.className}>{props.children}</div>
}

const bunTestModule: string = 'bun:test'
const { mock } = await import(bunTestModule)

mock.module('@/components/ui/sheet', () => ({
  Sheet: MockSheet,
  SheetContent: MockContainer,
  SheetHeader: MockContainer,
  SheetTitle: MockContainer,
}))

const { default: GroupDetailPanel } = await import('./group-detail-panel')

const group: MonitoringGroupWithHistory = {
  group_name: 'primary',
  is_online: true,
  online_channels: 1,
  total_channels: 1,
  availability_rate: 99.5,
  cache_hit_rate: 12.5,
  avg_frt: 120,
  avg_response_time: 240,
  first_response_time: 120,
  last_test_model: 'test-model',
  group_ratio: 1,
  updated_at: 1,
  history: [],
  aggregation_interval_minutes: 5,
}

async function renderPanel(): Promise<string> {
  const previousUser = useAuthStore.getState().auth.user
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.ADMIN,
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })

  try {
    return renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <GroupDetailPanel open group={group} onOpenChange={() => {}} />
        </QueryClientProvider>
      </I18nextProvider>
    )
  } finally {
    queryClient.clear()
    useAuthStore.getState().auth.setUser(previousUser)
  }
}

describe('group detail panel', () => {
  test('keeps its content region shrinkable and vertically scrollable', async () => {
    const markup = await renderPanel()
    const scrollRegion = markup.match(/<div class="([^"]*\bspace-y-5\b[^"]*)">/)

    assert.ok(scrollRegion, 'expected the group detail content region')
    const classNames = new Set(scrollRegion[1].split(/\s+/))
    assert.equal(classNames.has('overflow-y-auto'), true)
    assert.equal(classNames.has('flex-1'), true)
    assert.equal(classNames.has('min-h-0'), true)
  })
})
