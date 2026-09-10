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

import { updateSystemOptions } from './api.ts'

// 契约锁的是与后端 PUT /api/option/batch 的请求信封：一次请求、一个 options 数组、
// 顺序与调用方给定的顺序一致（后端按顺序 apply，首个失败即停止）。
// 若退化成逐项 PUT，17 项配置就是 17 次往返 —— 正是管理员保存卡顿的成因。
test('batch option save issues a single request carrying every key in order', async () => {
  const originalAdapter = api.defaults.adapter
  const requests: { url: string; method?: string; body: unknown }[] = []

  api.defaults.adapter = async (config) => {
    requests.push({
      url: config.url ?? '',
      method: config.method,
      body: JSON.parse(String(config.data)),
    })
    return {
      data: { success: true, message: '', data: { applied: [] } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }

  try {
    await updateSystemOptions({
      options: [
        { key: 'group_monitoring_setting.enabled', value: 'true' },
        { key: 'group_monitoring_setting.perf_card_top_n', value: '6' },
        { key: 'group_monitoring_setting.perf_card_groups', value: '["vip"]' },
      ],
    })
  } finally {
    api.defaults.adapter = originalAdapter
  }

  assert.equal(requests.length, 1)
  assert.equal(requests[0].url, '/api/option/batch')
  assert.equal(requests[0].method, 'put')
  assert.deepEqual(requests[0].body, {
    options: [
      { key: 'group_monitoring_setting.enabled', value: 'true' },
      { key: 'group_monitoring_setting.perf_card_top_n', value: '6' },
      { key: 'group_monitoring_setting.perf_card_groups', value: '["vip"]' },
    ],
  })
})

// 后端把"某一项校验不通过"表达成 HTTP 200 + success:false + data.failed，
// 而不是非 2xx。调用方必须能拿到 applied/failed 才能告诉管理员改到哪一项断了。
test('batch option save surfaces the applied prefix and the failing key', async () => {
  const originalAdapter = api.defaults.adapter

  api.defaults.adapter = async (config) => ({
    data: {
      success: false,
      message: '无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！',
      data: {
        applied: ['group_monitoring_setting.enabled'],
        failed: { key: 'GitHubOAuthEnabled', message: '缺少 Client Id' },
      },
    },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  })

  try {
    const res = await updateSystemOptions({
      options: [
        { key: 'group_monitoring_setting.enabled', value: 'true' },
        { key: 'GitHubOAuthEnabled', value: 'true' },
      ],
    })
    assert.equal(res.success, false)
    assert.deepEqual(res.data?.applied, ['group_monitoring_setting.enabled'])
    assert.equal(res.data?.failed?.key, 'GitHubOAuthEnabled')
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
