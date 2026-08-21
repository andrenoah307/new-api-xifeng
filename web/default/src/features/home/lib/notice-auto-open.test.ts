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

import { useNotificationStore } from '@/stores/notification-store'

function isClosedFor(date: string | null): boolean {
  const previousDate = useNotificationStore.getState().closedUntilDate
  useNotificationStore.setState({ closedUntilDate: date })

  try {
    return useNotificationStore.getState().isNoticeClosed()
  } finally {
    useNotificationStore.setState({ closedUntilDate: previousDate })
  }
}

const now = new Date()
const yesterday = new Date(now)
yesterday.setDate(yesterday.getDate() - 1)

const cases = [
  {
    name: 'rejects an empty notice',
    notice: '',
    noticeClosed: isClosedFor(null),
    blockedByModal: false,
    expected: false,
  },
  {
    name: 'rejects a whitespace-only notice',
    notice: '  \n ',
    noticeClosed: isClosedFor(null),
    blockedByModal: false,
    expected: false,
  },
  {
    name: 'rejects a notice closed today',
    notice: 'Maintenance',
    noticeClosed: isClosedFor(now.toDateString()),
    blockedByModal: false,
    expected: false,
  },
  {
    name: 'opens when the close date was yesterday',
    notice: 'Maintenance',
    noticeClosed: isClosedFor(yesterday.toDateString()),
    blockedByModal: false,
    expected: true,
  },
  {
    name: 'opens when no close date exists',
    notice: 'Maintenance',
    noticeClosed: isClosedFor(null),
    blockedByModal: false,
    expected: true,
  },
  {
    name: 'rejects a notice while a higher-priority modal is blocking',
    notice: 'Maintenance',
    noticeClosed: isClosedFor(null),
    blockedByModal: true,
    expected: false,
  },
]

for (const item of cases) {
  test(item.name, async () => {
    const { shouldAutoOpenNotice } = await import('./notice-auto-open')

    assert.equal(
      shouldAutoOpenNotice({
        notice: item.notice,
        noticeClosed: item.noticeClosed,
        blockedByModal: item.blockedByModal,
      }),
      item.expected
    )
  })
}
