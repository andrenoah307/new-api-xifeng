/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { Channel } from '../types'
import {
  normalizeExcludedUserGroups,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from './channel-form'

function channelWithSetting(setting: string): Channel {
  return {
    id: 1,
    type: 1,
    name: 'ch',
    group: 'default',
    models: 'gpt-4',
    channel_info: {},
    setting,
  } as unknown as Channel
}

describe('normalizeExcludedUserGroups', () => {
  test('drops blanks and duplicates while preserving order', () => {
    const cases: Array<{ input: string[] | undefined; expected: string[] }> = [
      { input: undefined, expected: [] },
      { input: [], expected: [] },
      { input: ['  ', ''], expected: [] },
      { input: [' vip ', 'vip', 'default'], expected: ['vip', 'default'] },
    ]
    for (const { input, expected } of cases) {
      assert.deepEqual(normalizeExcludedUserGroups(input), expected)
    }
  })
})

describe('excluded user groups round trip', () => {
  test('reads the list out of channel.setting', () => {
    const defaults = transformChannelToFormDefaults(
      channelWithSetting(
        JSON.stringify({ excluded_user_groups: ['vip', 42, '', 'pro'] })
      )
    )
    assert.deepEqual(defaults.excluded_user_groups, ['vip', 'pro'])
  })

  test('defaults to an empty list when the channel has no setting', () => {
    const defaults = transformChannelToFormDefaults(channelWithSetting(''))
    assert.deepEqual(defaults.excluded_user_groups, [])
  })

  test('writes the list back so editing here never erases it', () => {
    const defaults = transformChannelToFormDefaults(
      channelWithSetting(
        JSON.stringify({ proxy: 'p', excluded_user_groups: ['vip'] })
      )
    )
    const payload = transformFormDataToUpdatePayload(defaults, 1)
    const setting = JSON.parse(String(payload.setting))
    assert.deepEqual(setting.excluded_user_groups, ['vip'])
    assert.equal(setting.proxy, 'p')
  })

  test('omits the key entirely when nothing is excluded', () => {
    const defaults = transformChannelToFormDefaults(channelWithSetting('{}'))
    const payload = transformFormDataToUpdatePayload(defaults, 1)
    const setting = JSON.parse(String(payload.setting))
    assert.equal('excluded_user_groups' in setting, false)
  })
})
