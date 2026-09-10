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
import test from 'node:test'

import { api } from '@/lib/api'

import { getGroupHistoryBatch } from './api.ts'

async function withStubbedResponse<T>(
  payload: unknown,
  run: () => Promise<T>
): Promise<{ result: T; urls: string[] }> {
  const originalAdapter = api.defaults.adapter
  const urls: string[] = []
  api.defaults.adapter = async (config) => {
    urls.push(config.url ?? '')
    return {
      data: payload,
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  try {
    return { result: await run(), urls }
  } finally {
    api.defaults.adapter = originalAdapter
  }
}

// 契约锁的是后端 GetAdminMonitoringGroupsHistoryBatch 的真实信封：
// `data` 本身就是 {分组: 历史点[]} 这张表，`aggregation_interval_minutes`
// 是 `data` 的兄弟字段而不是它的子字段。
// 若按 `data.history` 去取，返回的永远是空表，管理页每张卡片的时序条会集体退化成
// "暂无历史数据"，而接口、控制台、后端日志全都是绿的 —— 属于静默失效。
test('batch group history reads the map and interval from the backend envelope', async () => {
  const { result, urls } = await withStubbedResponse(
    {
      success: true,
      message: '',
      data: {
        alpha: [{ recorded_at: 100, availability_rate: 99, avg_frt: 800, cache_hit_rate: 40 }],
        beta: [],
      },
      period_minutes: 720,
      aggregation_interval_minutes: 15,
    },
    () => getGroupHistoryBatch(true)
  )

  assert.deepEqual(urls, ['/api/monitoring/admin/group-history'])
  assert.deepEqual(Object.keys(result.history).sort(), ['alpha', 'beta'])
  assert.equal(result.history.alpha.length, 1)
  assert.equal(result.history.alpha[0].availability_rate, 99)
  assert.deepEqual(result.history.beta, [])
  assert.equal(result.intervalMinutes, 15)
})

// 后端可能因为分组监控未配置而返回 data: null，此时必须退回空表和默认间隔，
// 而不是抛错让整个看板白屏。
test('batch group history tolerates an empty payload', async () => {
  const { result } = await withStubbedResponse(
    { success: true, message: '', data: null },
    () => getGroupHistoryBatch(true)
  )

  assert.deepEqual(result.history, {})
  assert.equal(result.intervalMinutes, 5)
})

// 非管理员没有这个端点的权限，必须在客户端短路，不能发出注定 403 的请求。
test('batch group history never fires for non-admin viewers', async () => {
  const { result, urls } = await withStubbedResponse({}, () =>
    getGroupHistoryBatch(false)
  )

  assert.deepEqual(urls, [])
  assert.deepEqual(result.history, {})
  assert.equal(result.intervalMinutes, 5)
})
