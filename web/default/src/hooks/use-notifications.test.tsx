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
import { test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderToStaticMarkup } from 'react-dom/server'

import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from './use-notifications'

let capturedNotifications: ReturnType<typeof useNotifications> | null = null

function NotificationsProbe() {
  capturedNotifications = useNotifications()
  return <output />
}

test('useNotifications keeps its public return contract', () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['notice'], {
    success: true,
    data: '  Maintenance notice  ',
  })
  queryClient.setQueryData(['status'], {
    announcements_enabled: true,
    announcements: [{ id: 1, content: 'Platform update' }],
  })
  useNotificationStore.setState({
    lastReadNotice: '',
    readAnnouncementKeys: [],
  })

  try {
    renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <NotificationsProbe />
      </QueryClientProvider>
    )

    assert.ok(capturedNotifications)
    assert.deepEqual(Object.keys(capturedNotifications), [
      'notice',
      'announcements',
      'loading',
      'unreadCount',
      'unreadNoticeCount',
      'unreadAnnouncementsCount',
      'popoverOpen',
      'setPopoverOpen',
      'activeTab',
      'setActiveTab',
      'openPopover',
      'closePopover',
      'refetchNotice',
    ])
    assert.equal(capturedNotifications.notice, 'Maintenance notice')
    assert.equal(capturedNotifications.announcements.length, 1)
    assert.equal(capturedNotifications.loading, false)
    assert.equal(capturedNotifications.unreadNoticeCount, 1)
    assert.equal(capturedNotifications.unreadAnnouncementsCount, 1)
    assert.equal(capturedNotifications.unreadCount, 2)
    assert.equal(capturedNotifications.popoverOpen, false)
    assert.equal(capturedNotifications.activeTab, 'notice')
    assert.equal(typeof capturedNotifications.setPopoverOpen, 'function')
    assert.equal(typeof capturedNotifications.setActiveTab, 'function')
    assert.equal(typeof capturedNotifications.openPopover, 'function')
    assert.equal(typeof capturedNotifications.closePopover, 'function')
    assert.equal(typeof capturedNotifications.refetchNotice, 'function')
  } finally {
    capturedNotifications = null
    queryClient.clear()
  }
})
