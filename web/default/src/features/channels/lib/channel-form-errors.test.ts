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
import type { FieldErrors } from 'react-hook-form'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  type ChannelFormValues,
} from './channel-form'
import { hasAdvancedSettingsErrors } from './channel-form-errors'

describe('channel form errors', () => {
  test('detects validation errors in collapsed advanced JSON fields', () => {
    const errors = {
      param_override: {
        type: 'custom',
        message: 'Invalid JSON',
      },
    } satisfies FieldErrors<ChannelFormValues>

    assert.equal(hasAdvancedSettingsErrors(errors), true)
  })

  test('ignores validation errors outside advanced settings', () => {
    const errors = {
      name: {
        type: 'too_small',
        message: 'Name is required',
      },
    } satisfies FieldErrors<ChannelFormValues>

    assert.equal(hasAdvancedSettingsErrors(errors), false)
  })

  test('does not treat model mapping errors as advanced settings errors', () => {
    const errors = {
      model_mapping: {
        type: 'custom',
        message: 'Invalid model mapping',
      },
    } satisfies FieldErrors<ChannelFormValues>

    assert.equal(hasAdvancedSettingsErrors(errors), false)
  })

  test('classifies schema errors from invalid advanced JSON fields', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'OpenAI',
      type: 1,
      key: 'sk-test',
      models: 'gpt-4o',
      group: ['default'],
      param_override: '{',
    })

    assert.equal(result.success, false)
    if (result.success) return

    const errors = result.error.flatten().fieldErrors

    assert.ok(errors.param_override?.length)
    assert.equal(hasAdvancedSettingsErrors(errors), true)
  })
})
