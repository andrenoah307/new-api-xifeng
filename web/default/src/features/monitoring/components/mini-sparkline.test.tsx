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

import { segmentColor } from '../constants'
import MiniSparkline from './mini-sparkline'

async function render(series: (number | null)[]): Promise<string> {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <MiniSparkline series={series} />
    </I18nextProvider>
  )
}

function segmentTitles(markup: string): string[] {
  return [...markup.matchAll(/title="([^"]*)"/g)].map((m) => m[1])
}

function segmentBackgrounds(markup: string): string[] {
  return [...markup.matchAll(/background:([^;"]*)/g)].map((m) => m[1].trim())
}

describe('mini sparkline', () => {
  // 契约：后端固定返回 GroupModelSeriesSlots 个槽位，缩略图必须一比一渲染，
  // 不能压缩掉空槽 —— 否则时间轴会失真，两个模型的同一列不再是同一时刻。
  test('renders one segment per slot including the empty ones', async () => {
    const markup = await render([null, 100, null, 50, null])

    assert.equal(segmentBackgrounds(markup).length, 5)
  })

  // 契约：空槽位是"这段时间没有请求"，必须走无数据灰，
  // 不能落进成功率色阶被画成红色，那会把"没流量"谎报成"全挂了"。
  test('paints untouched slots with the no-data shade, never the failure color', async () => {
    const markup = await render([null, 100])
    const backgrounds = segmentBackgrounds(markup)

    assert.equal(backgrounds[0], segmentColor(null, null))
    assert.notEqual(backgrounds[0], segmentColor(0, null))
    assert.equal(backgrounds[1], segmentColor(100, null))
  })

  // 色阶必须复用分组时序条的 segmentColor，两处不能各写一套阈值，
  // 否则同一个成功率在页面上下会呈现不同颜色。
  test('reuses the group timeline color scale for every rate band', async () => {
    const rates = [100, 97, 90, 60, 10, 0]
    const markup = await render(rates)

    assert.deepEqual(
      segmentBackgrounds(markup),
      rates.map((r) => segmentColor(r, null))
    )
  })

  // 悬浮提示走原生 title，不引入 Tooltip 组件：单页最多 24 槽 × 数十个模型 × 数十个分组，
  // 每格都挂一个 Tooltip 会造出上万个组件实例。
  test('labels each slot with a native title carrying the rate', async () => {
    const markup = await render([null, 99.5])
    const titles = segmentTitles(markup)

    assert.equal(titles.length, 2)
    assert.match(titles[0], /No data available/)
    assert.match(titles[1], /99\.5%/)
  })

  test('renders nothing when the backend returned no series at all', async () => {
    assert.equal(segmentBackgrounds(await render([])).length, 0)
  })
})
