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
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  parseQuotaFromDollars,
  quotaUnitsToDollars,
  quotaUnitsToInputAmount,
} from './format'

const quotaCases = [1, 68_493, 684_932] as const

describe('quota input amount conversion', () => {
  test('round-trips editable amounts in tokens, USD, and CNY display modes', () => {
    const originalConfig = useSystemConfigStore.getState().config
    try {
      const modes = [
        {
          name: 'tokens',
          currency: {
            ...DEFAULT_CURRENCY_CONFIG,
            quotaDisplayType: 'TOKENS' as const,
            quotaPerUnit: 500_000,
            usdExchangeRate: 7.3,
          },
        },
        {
          name: 'USD',
          currency: {
            ...DEFAULT_CURRENCY_CONFIG,
            quotaDisplayType: 'USD' as const,
            quotaPerUnit: 500_000,
            usdExchangeRate: 1,
          },
        },
        {
          name: 'CNY',
          currency: {
            ...DEFAULT_CURRENCY_CONFIG,
            quotaDisplayType: 'CNY' as const,
            quotaPerUnit: 500_000,
            usdExchangeRate: 7.3,
          },
        },
      ] as const

      for (const mode of modes) {
        useSystemConfigStore.setState({
          config: { ...originalConfig, currency: mode.currency },
        })
        for (const quota of quotaCases) {
          assert.equal(
            parseQuotaFromDollars(quotaUnitsToInputAmount(quota)),
            quota,
            `${mode.name} quota ${quota}`
          )
        }
      }

      useSystemConfigStore.setState({
        config: {
          ...originalConfig,
          currency: {
            ...DEFAULT_CURRENCY_CONFIG,
            quotaDisplayType: 'CNY',
            quotaPerUnit: 500_000,
            usdExchangeRate: 7.3,
          },
        },
      })
      const cnyExact = quotaUnitsToDollars(68_493)
      const cnyInput = quotaUnitsToInputAmount(68_493)
      assert.ok(String(cnyInput).length < String(cnyExact).length)
      assert.equal(cnyInput, 1)

      useSystemConfigStore.setState({
        config: {
          ...originalConfig,
          currency: {
            ...DEFAULT_CURRENCY_CONFIG,
            quotaDisplayType: 'USD',
            quotaPerUnit: 500_000,
            usdExchangeRate: 1,
          },
        },
      })
      for (const quota of quotaCases) {
        assert.equal(
          quotaUnitsToInputAmount(quota),
          quotaUnitsToDollars(quota),
          `USD quota ${quota}`
        )
      }
    } finally {
      useSystemConfigStore.setState({ config: originalConfig })
    }
  })
})
