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
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import {
  ChannelRateLimitEditor,
  DEFAULT_CHANNEL_RATE_LIMIT,
  parseChannelRateLimit,
  serializeChannelRateLimit,
} from './channel-rate-limit-editor'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

describe('channel rate-limit defaults', () => {
  test('uses the backend defaults when no channel configuration exists', () => {
    assert.deepEqual(parseChannelRateLimit(undefined), {
      ...DEFAULT_CHANNEL_RATE_LIMIT,
    })
    assert.deepEqual(parseChannelRateLimit(''), {
      ...DEFAULT_CHANNEL_RATE_LIMIT,
    })
    assert.deepEqual(parseChannelRateLimit('{}'), {
      ...DEFAULT_CHANNEL_RATE_LIMIT,
    })
    assert.equal(DEFAULT_CHANNEL_RATE_LIMIT.queue_max_wait_ms, 2000)
    assert.equal(DEFAULT_CHANNEL_RATE_LIMIT.queue_depth, 20)
  })

  test('falls back to backend defaults for invalid or non-object settings', () => {
    for (const raw of ['not-json', 'null', '[]', '1']) {
      assert.deepEqual(parseChannelRateLimit(raw), {
        ...DEFAULT_CHANNEL_RATE_LIMIT,
      })
    }
  })

  test('preserves explicit stored values, including zeroes', () => {
    const stored = {
      enabled: true,
      rpm: 17,
      concurrency: 3,
      on_limit: 'queue' as const,
      queue_max_wait_ms: 0,
      queue_depth: 0,
    }

    assert.deepEqual(parseChannelRateLimit(JSON.stringify(stored)), stored)
  })

  test('round-trips a parsed configuration with backend-equivalent defaults', () => {
    const stored = {
      enabled: true,
      rpm: 12,
      concurrency: 2,
      on_limit: 'queue',
    }
    const parsed = parseChannelRateLimit(JSON.stringify(stored))
    const serialized = serializeChannelRateLimit(parsed)

    assert.deepEqual(JSON.parse(serialized), {
      ...DEFAULT_CHANNEL_RATE_LIMIT,
      ...stored,
    })
  })

  test('renders an input for the queue depth setting', () => {
    const form = {
      watch: () => undefined,
      setValue: () => undefined,
    }
    const markup = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(ChannelRateLimitEditor, { form: form as never })
      )
    )

    assert.match(markup, />Queue Depth</)
    assert.match(markup, /value="20"/)
  })
})
