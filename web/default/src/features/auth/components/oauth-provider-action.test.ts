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

import { runOAuthProviderAction } from './oauth-provider-action'

describe('OAuth provider action preflight', () => {
  test('does not start the provider action when preflight rejects it', () => {
    let actionCalls = 0

    runOAuthProviderAction(
      () => false,
      () => {
        actionCalls += 1
      }
    )

    assert.equal(actionCalls, 0)
  })

  test('starts the provider action when preflight accepts it', () => {
    let actionCalls = 0

    runOAuthProviderAction(
      () => true,
      () => {
        actionCalls += 1
      }
    )

    assert.equal(actionCalls, 1)
  })

  test('keeps existing behavior when no preflight is supplied', () => {
    let actionCalls = 0

    runOAuthProviderAction(undefined, () => {
      actionCalls += 1
    })

    assert.equal(actionCalls, 1)
  })
})
