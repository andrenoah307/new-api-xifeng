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

import {
  getGroupModelPerformance,
  getGroupHistoryBatch,
  getMonitoringGroups,
  getGroupHistory,
  getGroupDetail,
} from './api.ts'

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

test('monitoring applies cache counts to public/admin snapshots, history and channel detail', async () => {
  const sample = {
    cache_hit_rate: 40,
    cache_request_count: 10,
    cache_hit_request_count: 9,
  }
  for (const admin of [true, false]) {
    const groups = await withStubbedResponse(
      {
        data: [
          {
            ...sample,
            group_name: 'alpha',
            cache_hit_rate_1h: 95,
            cache_request_count_1h: 10,
            cache_hit_request_count_1h: 8,
          },
        ],
      },
      () => getMonitoringGroups(admin)
    )
    assert.equal(groups.result[0].cache_hit_rate, 90)
    assert.equal(groups.result[0].cache_hit_rate_1h, 95)
    const history = await withStubbedResponse({ data: [sample] }, () =>
      getGroupHistory('alpha', admin)
    )
    assert.equal(history.result.history[0].cache_hit_rate, 90)
  }
  const batch = await withStubbedResponse({ data: { alpha: [sample] } }, () =>
    getGroupHistoryBatch(true)
  )
  assert.equal(batch.result.history.alpha[0].cache_hit_rate, 90)
  const detail = await withStubbedResponse(
    { data: sample, channel_stats: [{ ...sample, channel_id: 1 }] },
    () => getGroupDetail('alpha')
  )
  assert.equal(detail.result.cache_hit_rate, 90)
  assert.equal(detail.result.channel_stats[0].cache_hit_rate, 90)
  assert.equal(sample.cache_hit_rate, 40)
})

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
        alpha: [
          {
            recorded_at: 100,
            availability_rate: 99,
            avg_frt: 800,
            cache_hit_rate: 40,
          },
        ],
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

// 模型性能过去只有管理员端点，普通用户的看板整块缺失。放开后两侧共用同一段
// 取数与渲染，唯一的差别是端点前缀——与 groups / history 已有的 admin|public
// 约定保持一致，不引入第二套写法。
test('model performance picks the endpoint by viewer role', async () => {
  const payload = {
    success: true,
    message: '',
    data: { enabled: true, groups: {} },
  }
  const asAdmin = await withStubbedResponse(payload, () =>
    getGroupModelPerformance(true)
  )
  const asUser = await withStubbedResponse(payload, () =>
    getGroupModelPerformance(false)
  )

  assert.deepEqual(asAdmin.urls, ['/api/monitoring/admin/model-performance'])
  assert.deepEqual(asUser.urls, ['/api/monitoring/public/model-performance'])
})

// 公开端点剥掉 request_count（真实业务量），其余七个服务质量字段原样保留。
// 前端必须能在缺字段的情况下正常渲染，否则脱敏一上线普通用户就白屏。
test('model performance survives the desensitized payload without request counts', async () => {
  const { result } = await withStubbedResponse(
    {
      success: true,
      message: '',
      data: {
        enabled: true,
        window_hours: 24,
        top_n: 6,
        show_all_models: false,
        enabled_groups: ['alpha'],
        groups: {
          alpha: [
            {
              model_name: 'gpt-5.6-luna',
              success_rate: 99.2,
              avg_latency_ms: 1200,
              avg_ttft_ms: 300,
              has_ttft: true,
              avg_tps: 45,
              series: [null, 99],
            },
          ],
        },
      },
    },
    () => getGroupModelPerformance(false)
  )

  const model = result.groups.alpha[0]
  assert.equal(model.request_count, undefined)
  assert.equal(model.model_name, 'gpt-5.6-luna')
  assert.equal(model.success_rate, 99.2)
  assert.deepEqual(model.series, [null, 99])
})
