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
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import type { GroupModelPerf, MonitoringGroupWithHistory } from '../api'
import GroupStatusCard from './group-status-card'

const group: MonitoringGroupWithHistory = {
  group_name: 'vip_3',
  is_online: true,
  online_channels: 8,
  total_channels: 9,
  availability_rate: 99.5,
  cache_hit_rate: 12.4,
  avg_frt: 1200,
  avg_response_time: 2400,
  first_response_time: 1200,
  last_test_model: 'probe-model',
  group_ratio: 7,
  updated_at: 1,
  history: [],
  aggregation_interval_minutes: 5,
}

const models: GroupModelPerf[] = [
  {
    model_name: 'gpt-5.6-luna',
    request_count: 100,
    success_rate: 99,
    avg_latency_ms: 1200,
    avg_ttft_ms: 300,
    has_ttft: true,
    avg_tps: 45,
  },
]

async function renderCard(props: {
  variant?: 'grid' | 'wide'
  withModelPerformance: boolean
}): Promise<string> {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <GroupStatusCard
          group={group}
          variant={props.variant}
          modelPerformance={
            props.withModelPerformance
              ? { models, showAll: true, topN: 6, windowHours: 24 }
              : undefined
          }
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

function occurrences(markup: string, needle: string): number {
  return markup.split(needle).length - 1
}

describe('group status card layout', () => {
  // 回归护栏：置顶宽卡片曾把四段内容（头部/时序条/底部统计/模型性能）
  // 直接挂在一个两列 Grid 下，Grid 逐个换行摆放，"左分组右模型"从未生效。
  // 四列骨架要求这个 Grid 只有两个直接子元素，各自恰好出现一次。
  test('splits the wide card into exactly two grid columns', async () => {
    const markup = await renderCard({ variant: 'wide', withModelPerformance: true })
    assert.equal(occurrences(markup, 'data-perf-col="group"'), 1)
    assert.equal(occurrences(markup, 'data-perf-col="models"'), 1)
    // 模型性能表必须落在右列之内，而不是与底部统计并排
    assert.ok(
      markup.indexOf('data-perf-col="models"') < markup.indexOf('<table'),
      'model performance table must render inside the models column'
    )
    // 两列各自都要有内容：左列分组名、右列模型名
    assert.ok(markup.includes('vip_3'))
    assert.ok(markup.includes('gpt-5.6-luna'))
  })

  test('keeps the single-column flow when the wide card has no model performance', async () => {
    const markup = await renderCard({ variant: 'wide', withModelPerformance: false })
    assert.equal(occurrences(markup, 'data-perf-col'), 0)
    assert.ok(markup.includes('vip_3'))
  })

  test('stacks model performance below the card in the multi-column grid variant', async () => {
    const markup = await renderCard({ variant: 'grid', withModelPerformance: true })
    assert.equal(occurrences(markup, 'data-perf-col'), 0)
    assert.ok(markup.includes('gpt-5.6-luna'))
  })
})

// 四列骨架由三段 class 共同决定：Grid 的 grid-cols-N、左列 col-span-a、右列
// col-span-b。三者任意一段的断点或数字被单独改动，浏览器都会静默给出错误
// 布局（空轨道、断点错位、右列反被挤压），而 SSR 结构断言依旧全绿。
function perfColClass(markup: string, col: string): string {
  const tag = markup.match(new RegExp(`<div[^>]*data-perf-col="${col}"[^>]*>`))?.[0] ?? ''
  return tag.match(/class="([^"]*)"/)?.[1] ?? ''
}

function span(className: string): { breakpoint: string; cols: number } {
  const m = className.match(/@([\w[\]]+)\/perfcard:col-span-(\d+)/)
  return { breakpoint: m?.[1] ?? '', cols: m ? Number(m[2]) : 0 }
}

describe('group status card split ratio', () => {
  test('splits the grid tracks exactly between the two columns at one breakpoint', async () => {
    const markup = await renderCard({ variant: 'wide', withModelPerformance: true })
    const grid = markup.match(/@([\w[\]]+)\/perfcard:grid-cols-(\d+)/)
    assert.ok(grid, 'the wide card must define its multi-column tracks')
    const left = span(perfColClass(markup, 'group'))
    const right = span(perfColClass(markup, 'models'))
    // 左列可以省略 col-span-1（Grid 默认跨 1 轨），此时按 1 计
    const leftCols = left.cols || 1
    assert.equal(right.breakpoint, grid[1], 'columns must span at the same breakpoint as the tracks')
    if (left.cols) assert.equal(left.breakpoint, grid[1])
    assert.equal(leftCols + right.cols, Number(grid[2]), 'the two columns must consume every track')
    // 信息密度：模型性能表承载 6 列 × 最多 20 行，必须拿到不少于分组卡片的宽度
    assert.ok(right.cols > leftCols, 'the model table must be wider than the group card')
  })
})
