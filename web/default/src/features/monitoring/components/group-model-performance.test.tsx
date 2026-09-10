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

// 每个模型一行、单元格顺序为 [模型名, 首字, 延迟, TPS, 成功率, 走势]，
// 这是表格化后的可观测契约。断言只依赖 <tbody>/<tr>/<td> 结构，
// 不锁定 class、嵌套层级或 Tailwind 工具类。
function rowCells(markup: string): string[][] {
  const start = markup.indexOf('<tbody>')
  if (start < 0) return []
  const body = markup.slice(start, markup.indexOf('</tbody>'))
  return [...body.matchAll(/<tr[^>]*>(.*?)<\/tr>/g)].map((row) =>
    [...row[1].matchAll(/<td[^>]*>(.*?)<\/td>/g)].map((cell) =>
      cell[1].replace(/<[^>]*>/g, '')
    )
  )
}

function renderedModelNames(markup: string): string[] {
  return rowCells(markup).map((cells) => cells[0])
}

function occurrences(markup: string, needle: string): number {
  return markup.split(needle).length - 1
}

// 走势列的宽度契约：auto 表格布局下，百分比宽度（w-full）与 flex-1 子项
// 对列的固有宽度贡献为 0，列会塌到只剩 gap 的宽度。因此"给单元格内容一个
// 固定宽度"不是样式偏好，而是这一列能否被看见的唯一机制。
function trendHeaderClass(markup: string): string {
  const head = markup.slice(markup.indexOf('<thead>'), markup.indexOf('</thead>'))
  const th = [...head.matchAll(/<th([^>]*)>(.*?)<\/th>/g)].find((m) => m[2].includes('Trend'))
  return th ? (th[1].match(/class="([^"]*)"/)?.[1] ?? '') : ''
}

function trendCell(markup: string): { className: string; inner: string } {
  const body = markup.slice(markup.indexOf('<tbody>'), markup.indexOf('</tbody>'))
  const row = body.match(/<tr[^>]*>(.*?)<\/tr>/)?.[1] ?? ''
  const cells = [...row.matchAll(/<td([^>]*)>(.*?)<\/td>/g)]
  const last = cells[cells.length - 1]
  return last
    ? { className: last[1].match(/class="([^"]*)"/)?.[1] ?? '', inner: last[2] }
    : { className: '', inner: '' }
}

function containerBreakpoint(className: string): string {
  return className.match(/@([\w[\]]+)\/perfcols:table-cell/)?.[1] ?? ''
}

describe('group model performance', () => {
  // 契约：窗口内没有任何一次首字采样时后端返回 avg_ttft_ms=0，
  // 必须渲染"—"。若改成直出数值会显示 "0ms"，运营会误判为极快。
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
    assert.deepEqual(rowCells(markup)[0].slice(0, 5), [
      'idle',
      '—',
      '—',
      '—',
      '99.00%',
    ])
    assert.doesNotMatch(markup, /0ms/)
  })

  test('formats sampled metrics instead of dashing them out', async () => {
    const markup = await render({
      models: [perf({ model_name: 'busy', avg_ttft_ms: 300, avg_latency_ms: 1200, avg_tps: 45 })],
      showAll: false,
      topN: 6,
    })
    assert.deepEqual(rowCells(markup)[0].slice(0, 5), [
      'busy',
      '300ms',
      '1.20s',
      '45.0 t/s',
      '99.00%',
    ])
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

  // 分组存在但窗口内无任何请求时后端返回空数组，必须给出占位而不是渲染一个空表格。
  test('shows a placeholder when the group reported no model traffic', async () => {
    const markup = await render({ models: [], showAll: false, topN: 6 })
    assert.match(markup, /No model performance data/)
    assert.deepEqual(renderedModelNames(markup), [])
  })

  // 表格化的全部收益都建立在"标签只写一次"上：指标名进表头，行内只留数值。
  // 一旦有人把标签写回每一行，这条断言会随模型数增加而失败。
  test('states each metric label once in the header regardless of row count', async () => {
    const two = await render({
      models: ['a', 'b'].map((n) => perf({ model_name: n })),
      showAll: true,
      topN: 6,
    })
    const four = await render({
      models: ['a', 'b', 'c', 'd'].map((n) => perf({ model_name: n })),
      showAll: true,
      topN: 6,
    })
    for (const label of [
      'Model',
      'First token latency short',
      'Latency',
      'Throughput short',
      'Model success rate short',
      'Trend',
    ]) {
      assert.equal(occurrences(two, `>${label}<`), 1, `${label} must appear once`)
      assert.equal(occurrences(four, `>${label}<`), 1, `${label} must not repeat per row`)
    }
    assert.equal(rowCells(two).length, 2)
    assert.equal(rowCells(four).length, 4)
  })

  // 生产里模型名最长 35 字符。截断是 CSS 行为，全名必须始终留在 DOM 里
  // （文本 + title），否则布局改动会静默吞掉管理员唯一的识别依据。
  test('keeps the full model name in the row even when it will be truncated', async () => {
    const longName = 'claude-sonnet-4-5-20260101-thinking'
    const markup = await render({
      models: [perf({ model_name: longName })],
      showAll: true,
      topN: 6,
    })
    assert.equal(renderedModelNames(markup)[0], longName)
    assert.ok(markup.includes(`title="${longName}"`))
  })
})

describe('group model performance trend column', () => {
  async function renderOne(): Promise<string> {
    return render({
      models: [perf({ model_name: 'gpt-5.6-luna', series: Array(24).fill(99) })],
      showAll: true,
      topN: 6,
    })
  }

  // 表头与单元格是同一列的两半。只改其中一个的显隐断点，浏览器会渲染出
  // "有表头没数据"或"有数据没表头"的错位表格，而 tsc 与其余测试全绿。
  test('reveals the trend header and its cells at the same container breakpoint', async () => {
    const markup = await renderOne()
    const head = containerBreakpoint(trendHeaderClass(markup))
    const cell = containerBreakpoint(trendCell(markup).className)
    assert.notEqual(head, '', 'trend header must declare a container breakpoint')
    assert.equal(head, cell)
  })

  // 回归护栏：走势列曾只在 <th> 上写 w-[76px]，而 auto 布局按单元格内容定列宽，
  // 表头上的宽度声明不参与，24 段被压成约 1.7px 的噪点。宽度必须落在单元格内容上。
  test('gives the trend cell content an intrinsic width instead of a percentage', async () => {
    const { inner } = trendCell(await renderOne())
    // 只认独立的 w-<数字>；min-w-0 / max-w-0 这类收缩声明不构成宽度契约
    assert.match(
      inner,
      /class="(?:[^"]*\s)?w-\d/,
      'trend cell must wrap the sparkline in a fixed-width box'
    )
  })

  // 表头单独声明像素宽度在 auto 布局下是无效声明，留着会误导后续维护者
  // 以为这一列已经有宽度契约了。
  test('does not declare a dead pixel width on the trend header', async () => {
    assert.doesNotMatch(trendHeaderClass(await renderOne()), /w-\[\d+px\]/)
  })
})
