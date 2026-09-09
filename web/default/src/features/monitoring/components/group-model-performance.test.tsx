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

import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import type { GroupModelPerf } from '../api'
import GroupModelPerformance from './group-model-performance'

function perf(overrides: Partial<GroupModelPerf> & { model_name: string }): GroupModelPerf {
  return {
    request_count: 100,
    success_rate: 99,
    avg_latency_ms: 1200,
    avg_ttft_ms: 300,
    has_ttft: true,
    avg_tps: 45,
    ...overrides,
  }
}

async function render(props: {
  models: GroupModelPerf[]
  showAll: boolean
  topN: number
}): Promise<string> {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <GroupModelPerformance {...props} windowHours={24} />
    </I18nextProvider>
  )
}

function renderedModelNames(markup: string): string[] {
  return [...markup.matchAll(/<div class="truncate font-medium" title="([^"]*)">/g)].map((m) => m[1])
}

describe('group model performance', () => {
  // 契约：窗口内没有任何一次首字采样时后端返回 avg_ttft_ms=0，
  // 卡片必须渲染"—"。若改成直出数值会显示 "0ms"，运营会误判为极快。
  // 延迟/TPS/首字三个指标共用同一条"无数据即 —"规则。
  test('renders a dash for every metric the window never sampled', async () => {
    const markup = await render({
      models: [
        perf({
          model_name: 'idle',
          has_ttft: false,
          avg_ttft_ms: 0,
          avg_tps: 0,
          avg_latency_ms: 0,
        }),
      ],
      showAll: false,
      topN: 6,
    })
    assert.match(markup, /First token latency —/)
    assert.match(markup, /Latency —/)
    assert.match(markup, /Throughput short —/)
    assert.doesNotMatch(markup, /0ms/)
  })

  test('formats sampled metrics instead of dashing them out', async () => {
    const markup = await render({
      models: [perf({ model_name: 'busy', avg_ttft_ms: 300, avg_latency_ms: 1200, avg_tps: 45 })],
      showAll: false,
      topN: 6,
    })
    assert.match(markup, /First token latency 300ms/)
    assert.match(markup, /Latency 1\.20s/)
    assert.match(markup, /Throughput short 45\.0 t\/s/)
  })

  // 契约：topN 是渲染上限，超出部分折叠。折叠计数必须是"被隐藏的数量"，
  // 而不是"总数"，否则管理员点开后发现数量对不上。
  test('folds to topN and offers to expand the remainder', async () => {
    const markup = await render({
      models: ['a', 'b', 'c', 'd', 'e'].map((n) => perf({ model_name: n })),
      showAll: false,
      topN: 2,
    })
    assert.deepEqual(renderedModelNames(markup), ['a', 'b'])
    assert.match(markup, /Expand all \(3\)/)
  })

  test('renders every model and hides the toggle when show-all is configured', async () => {
    const markup = await render({
      models: ['a', 'b', 'c', 'd', 'e'].map((n) => perf({ model_name: n })),
      showAll: true,
      topN: 2,
    })
    assert.deepEqual(renderedModelNames(markup), ['a', 'b', 'c', 'd', 'e'])
    assert.doesNotMatch(markup, /Expand all/)
  })

  test('hides the toggle when the model count already fits within topN', async () => {
    const markup = await render({
      models: ['a', 'b'].map((n) => perf({ model_name: n })),
      showAll: false,
      topN: 6,
    })
    assert.deepEqual(renderedModelNames(markup), ['a', 'b'])
    assert.doesNotMatch(markup, /Expand all/)
  })

  // 分组存在但窗口内无任何请求时后端返回空数组，必须给出占位而不是渲染一个空网格。
  test('shows a placeholder when the group reported no model traffic', async () => {
    const markup = await render({ models: [], showAll: false, topN: 6 })
    assert.match(markup, /No model performance data/)
    assert.deepEqual(renderedModelNames(markup), [])
  })
})
