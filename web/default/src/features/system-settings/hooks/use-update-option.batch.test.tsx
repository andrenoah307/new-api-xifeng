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
import { renderToStaticMarkup } from 'react-dom/server'

import { api } from '@/lib/api'
import type { UpdateOptionRequest } from '../types'
import { useUpdateOptions } from './use-update-option'

// useUpdateOptions 的全部价值就是"一次往返"。如果它退化成在 hook 内部循环调用
// 单项接口，api 层测试依然全绿（那层测的是 updateSystemOptions 本身），
// 设置页也照常保存成功 —— 只有网络面板能看出 17 项配置发了 17 次 PUT。
// 因此这条契约必须锁在 hook 这一层。
describe('useUpdateOptions request shape', () => {
  test('sends exactly one PUT /api/option/batch for the whole batch', async () => {
    const originalAdapter = api.defaults.adapter
    const requests: { url: string; body: unknown }[] = []

    api.defaults.adapter = async (config) => {
      requests.push({ url: config.url ?? '', body: JSON.parse(String(config.data)) })
      return {
        data: { success: true, message: '', data: { applied: [] } },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const options: UpdateOptionRequest[] = [
      { key: 'group_monitoring_setting.enabled', value: 'true' },
      { key: 'group_monitoring_setting.perf_card_enabled', value: 'true' },
      { key: 'group_monitoring_setting.perf_card_groups', value: '["vip"]' },
    ]

    let fired = false
    function Probe() {
      const mutation = useUpdateOptions()
      if (!fired) {
        fired = true
        mutation.mutate({ options })
      }
      return null
    }

    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })

    try {
      renderToStaticMarkup(
        <QueryClientProvider client={queryClient}>
          <Probe />
        </QueryClientProvider>
      )
      await new Promise((resolve) => setTimeout(resolve, 0))
    } finally {
      api.defaults.adapter = originalAdapter
    }

    assert.equal(requests.length, 1)
    assert.equal(requests[0].url, '/api/option/batch')
    assert.deepEqual(requests[0].body, { options })
  })
})
