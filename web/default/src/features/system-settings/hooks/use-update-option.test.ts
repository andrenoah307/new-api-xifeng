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

describe('useUpdateOptions batch query invalidation', () => {
  // 契约：批量保存只失效一次。逐项调用会让 N 项配置触发 N 次 system-options 失效，
  // 每次都重新拉一遍整张配置表 —— 这正是"逐分组请求"体验差的根源。
  test('invalidates system-options exactly once for a whole batch', () => {
    const { queryClient, queryKeys } = recordInvalidations()

    invalidateOptionQueries(queryClient, [
      'group_monitoring_setting.enabled',
      'group_monitoring_setting.perf_card_enabled',
      'group_monitoring_setting.perf_card_groups',
    ])

    assert.deepEqual(queryKeys, [['system-options']])
  })

  // 批量里只要有一项影响 status，就必须失效 status；但无论命中几项都只失效一次。
  test('invalidates status once when the batch touches any status-related key', () => {
    const { queryClient, queryKeys } = recordInvalidations()

    invalidateOptionQueries(queryClient, [
      'UnrelatedOption',
      'RateLimitCapacityCardEnabled',
      'console_setting.faq_enabled',
    ])

    assert.deepEqual(queryKeys, [['system-options'], ['status']])
  })

  test('leaves status alone when no key in the batch is status-related', () => {
    const { queryClient, queryKeys } = recordInvalidations()

    invalidateOptionQueries(queryClient, ['UnrelatedOption', 'AnotherOption'])

    assert.deepEqual(queryKeys, [['system-options']])
  })

  // 空批次不应该发出任何请求，也就不该失效任何缓存。
  test('does nothing for an empty batch', () => {
    const { queryClient, queryKeys } = recordInvalidations()

    invalidateOptionQueries(queryClient, [])

    assert.deepEqual(queryKeys, [])
  })
})
