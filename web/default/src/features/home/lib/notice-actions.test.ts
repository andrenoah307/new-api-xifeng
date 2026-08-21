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

test('close for today writes the current local date without marking notice read', async () => {
  const { closeNoticeForToday } = await import('./notice-auto-open')
  const currentDate = new Date(2026, 7, 21, 23, 59, 59)
  let state = {
    closedUntilDate: null as string | null,
    lastReadNotice: 'still-unread',
  }
  let closed = false

  closeNoticeForToday({
    getCurrentDate: () => currentDate,
    setClosedUntilDate: (date) => {
      state = { ...state, closedUntilDate: date }
    },
    close: () => {
      closed = true
    },
  })

  assert.equal(state.closedUntilDate, currentDate.toDateString())
  assert.equal(state.lastReadNotice, 'still-unread')
  assert.equal(closed, true)
})

test('close for today still closes when persistence throws', async () => {
  const { closeNoticeForToday } = await import('./notice-auto-open')
  let closed = false

  assert.throws(
    () =>
      closeNoticeForToday({
        getCurrentDate: () => new Date(2026, 7, 21, 12),
        setClosedUntilDate: () => {
          throw new Error('storage unavailable')
        },
        close: () => {
          closed = true
        },
      }),
    /storage unavailable/
  )
  assert.equal(closed, true)
})
