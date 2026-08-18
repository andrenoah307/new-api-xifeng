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

import type { QueryClient } from '@tanstack/react-query'

import { invalidateOptionQueries } from './use-update-option'

const statusRelatedKeys = [
  'RateLimitCapacityCardEnabled',
  'ModelNameRPMRateLimit',
  'console_setting.api_info_enabled',
  'console_setting.uptime_kuma_enabled',
  'console_setting.announcements_enabled',
  'console_setting.faq_enabled',
]

function recordInvalidations(): {
  queryClient: QueryClient
  queryKeys: unknown[][]
} {
  const queryKeys: unknown[][] = []
  const queryClient = {
    invalidateQueries: ({ queryKey }: { queryKey: unknown[] }) => {
      queryKeys.push(queryKey)
      return Promise.resolve()
    },
  } as unknown as QueryClient
  return { queryClient, queryKeys }
}

describe('useUpdateOption query invalidation', () => {
  for (const key of statusRelatedKeys) {
    test(`invalidates status once after saving ${key}`, () => {
      const { queryClient, queryKeys } = recordInvalidations()

      invalidateOptionQueries(queryClient, key)

      assert.deepEqual(queryKeys, [['system-options'], ['status']])
    })
  }

  test('does not invalidate status for an unrelated option', () => {
    const { queryClient, queryKeys } = recordInvalidations()

    invalidateOptionQueries(queryClient, 'UnrelatedOption')

    assert.deepEqual(queryKeys, [['system-options']])
  })
})
