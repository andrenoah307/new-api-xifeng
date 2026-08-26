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

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToUpdatePayload,
} from '../lib/channel-form'
import { hasSensitiveFormChanges } from './use-channel-mutate-form'

const customSensitiveFields = [
  'error_filter_rules',
  'pressure_cooling',
  'channel_rate_limit',
  'risk_control_headers',
] as const

describe('channel sensitive form change detection', () => {
  for (const field of customSensitiveFields) {
    test(`blocks a change to ${field} without sensitive-write permission`, () => {
      assert.equal(
        hasSensitiveFormChanges({ [field]: true }),
        true,
        `${field} must be treated as sensitive`
      )
    })
  }

  test('blocks the existing setting field as well', () => {
    assert.equal(hasSensitiveFormChanges({ setting: true }), true)
  })

  test('allows a non-sensitive name-only edit', () => {
    assert.equal(hasSensitiveFormChanges({ name: true }), false)
  })

  test('ignores untouched sensitive fields', () => {
    assert.equal(
      hasSensitiveFormChanges({
        error_filter_rules: false,
        pressure_cooling: undefined,
        channel_rate_limit: null,
        risk_control_headers: '',
      }),
      false
    )
  })

  test('blocks sensitive submits before a request but allows name-only edits', () => {
    let requestCount = 0
    let deniedCount = 0

    const submit = (dirtyFields: Record<string, unknown>) => {
      if (hasSensitiveFormChanges(dirtyFields)) {
        deniedCount++
        return
      }
      requestCount++
    }

    for (const field of customSensitiveFields) {
      submit({ [field]: true })
    }
    assert.equal(deniedCount, customSensitiveFields.length)
    assert.equal(requestCount, 0)

    submit({ name: true })
    assert.equal(requestCount, 1)
  })

  test('keeps all custom settings in the sensitive update payload', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'gateway',
      models: 'gpt-4o',
      pressure_cooling: JSON.stringify({ enabled: true }),
      channel_rate_limit: JSON.stringify({ enabled: true, rpm: 12 }),
      error_filter_rules: JSON.stringify([{ action: 'retry' }]),
      risk_control_headers: JSON.stringify([
        { name: 'X-User', source: 'username', value: '' },
      ]),
    }

    const payload = transformFormDataToUpdatePayload(formData, 42)
    assert.ok(payload.setting)
    assert.deepEqual(JSON.parse(payload.setting), {
      force_format: false,
      thinking_to_content: false,
      proxy: '',
      pass_through_body_enabled: false,
      strip_request_id: false,
      system_prompt: '',
      system_prompt_override: false,
      pressure_cooling: { enabled: true },
      rate_limit: { enabled: true, rpm: 12 },
      error_filter_rules: [{ action: 'retry' }],
      risk_control_headers: [{ name: 'X-User', source: 'username', value: '' }],
    })
  })
})
