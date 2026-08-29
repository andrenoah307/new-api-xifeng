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
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { useCommonLogsColumns } from './columns/common-logs-columns'
import { BillingBreakdown, TokenBreakdown } from './dialogs/details-dialog'
import { MobileTokensField } from './usage-logs-mobile-card'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

function createLog(
  other: LogOtherData,
  promptTokens = 100,
  completionTokens = 20
): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    username: '',
    token_name: '',
    model_name: 'test-model',
    quota: 0,
    prompt_tokens: promptTokens,
    completion_tokens: completionTokens,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: '',
    upstream_request_id: '',
  }
}

function DesktopTokensCell(props: { log: UsageLog }) {
  const columns = useCommonLogsColumns(false)
  const tokensColumn = columns.find(
    (column) => 'accessorKey' in column && column.accessorKey === 'prompt_tokens'
  )
  if (!tokensColumn || typeof tokensColumn.cell !== 'function') {
    throw new Error('Tokens column cell is unavailable')
  }

  const renderCell = tokensColumn.cell as (context: {
    row: { original: UsageLog }
  }) => ReactNode
  return <>{renderCell({ row: { original: props.log } })}</>
}

function render(component: ReactNode): string {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{component}</I18nextProvider>
  )
}

function extractCacheLabels(markup: string): string[] {
  return [...markup.matchAll(/aria-label="(Cache (?:Read|Write) [^"]+)"/g)].map(
    (match) => match[1]
  )
}

function extractListWriteTokens(markup: string): number {
  const match = markup.match(/aria-label="Cache Write ([\d,]+)"/)
  assert.ok(match, 'expected a cache-write group in the Tokens cell')
  return Number(match[1].replaceAll(',', ''))
}

function extractDetailValue(markup: string, label: string): number {
  const labelEnd = markup.indexOf(`>${label}</span>`)
  if (labelEnd === -1) return 0

  const value = markup
    .slice(labelEnd + label.length + '</span>'.length + 1)
    .match(/<span[^>]*>([\d,]+)<\/span>/)
  assert.ok(value, `expected a numeric value after ${label}`)
  return Number(value[1].replaceAll(',', ''))
}

function sumDetailWriteTokens(markup: string): number {
  return ['Cache Write', 'Cache Write (5m)', 'Cache Write (1h)'].reduce(
    (total, label) => total + extractDetailValue(markup, label),
    0
  )
}

function textContent(markup: string): string {
  return markup.replaceAll(/<[^>]+>/g, '').trim()
}

describe('cache token rendering', () => {
  const writeCases: Array<{
    name: string
    other: LogOtherData
    expectedWriteTotal: number
  }> = [
    {
      name: 'total creation exceeds split buckets',
      other: {
        cache_creation_tokens: 30,
        cache_creation_tokens_5m: 10,
        cache_creation_tokens_1h: 5,
      },
      expectedWriteTotal: 30,
    },
    {
      name: 'split buckets exceed total creation',
      other: {
        cache_creation_tokens: 10,
        cache_creation_tokens_5m: 8,
        cache_creation_tokens_1h: 7,
      },
      expectedWriteTotal: 15,
    },
    {
      name: 'creation has no split buckets',
      other: { cache_creation_tokens: 30 },
      expectedWriteTotal: 30,
    },
  ]

  for (const fixture of writeCases) {
    test(`keeps list and detail write totals aligned when ${fixture.name}`, () => {
      const log = createLog(fixture.other)
      const listMarkup = render(<DesktopTokensCell log={log} />)
      const detailsMarkup = render(
        <TokenBreakdown log={log} other={fixture.other} />
      )
      const listWriteTokens = extractListWriteTokens(listMarkup)
      const detailWriteTokens = sumDetailWriteTokens(detailsMarkup)

      assert.equal(listWriteTokens, fixture.expectedWriteTotal)
      assert.equal(detailWriteTokens, fixture.expectedWriteTotal)
      assert.equal(listWriteTokens, detailWriteTokens)
    })
  }

  test('desktop and mobile use the same accessible cache groups', () => {
    const log = createLog({
      cache_tokens: 1_234,
      cache_creation_tokens: 30,
      cache_creation_tokens_5m: 10,
      cache_creation_tokens_1h: 5,
    })
    const desktopLabels = extractCacheLabels(
      render(<DesktopTokensCell log={log} />)
    )
    const mobileLabels = extractCacheLabels(render(<MobileTokensField log={log} />))

    assert.deepEqual(desktopLabels, ['Cache Read 1,234', 'Cache Write 30'])
    assert.deepEqual(mobileLabels, desktopLabels)
  })

  test('cache-only logs remain visible in desktop, mobile, and details', () => {
    const other = { cache_creation_tokens: 25 }
    const log = createLog(other, 0, 0)
    const desktopMarkup = render(<DesktopTokensCell log={log} />)
    const mobileMarkup = render(<MobileTokensField log={log} />)
    const detailsMarkup = render(<TokenBreakdown log={log} other={other} />)

    assert.equal(extractListWriteTokens(desktopMarkup), 25)
    assert.equal(extractListWriteTokens(mobileMarkup), 25)
    assert.equal(extractDetailValue(detailsMarkup, 'Cache Write'), 25)
  })

  test('logs without input, output, or cache tokens keep their empty states', () => {
    const other = {}
    const log = createLog(other, 0, 0)

    assert.equal(textContent(render(<DesktopTokensCell log={log} />)), '-')
    assert.equal(textContent(render(<MobileTokensField log={log} />)), '-')
    assert.equal(render(<TokenBreakdown log={log} other={other} />), '')
  })

  test('shows OpenAI cache read and creation unit prices in billing details', () => {
    const other: LogOtherData = {
      model_price: -1,
      model_ratio: 1,
      completion_ratio: 1,
      group_ratio: 1,
      cache_tokens: 40,
      cache_ratio: 0.1,
      cache_creation_tokens: 10,
      cache_creation_ratio: 1.25,
    }
    const markup = render(
      <BillingBreakdown
        log={createLog(other, 100, 7)}
        other={other}
        isAdmin={false}
      />
    )
    const content = textContent(markup)

    assert.match(content, /Cache Read\$0\.2\/M/)
    assert.match(content, /Cache Write\$2\.5\/M/)
    assert.doesNotMatch(content, /Cache Read.*undefined/)
  })
})
