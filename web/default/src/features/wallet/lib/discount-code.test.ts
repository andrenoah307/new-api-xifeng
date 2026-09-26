import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { exceedsDiscountCodeLimit } from './discount-code'

describe('discount code recharge limit in user input units', () => {
  for (const [amount, limit, exceeded] of [
    [10001, 0, false],
    [10000, 10000, false],
    [10001, 10000, true],
    [0, 10000, false],
    [5000001, 5000000, true],
    [10000.1, 10000, true],
  ] as const) {
    test(`amount ${amount}, limit ${limit}: exceeded ${exceeded}`, () => {
      assert.equal(exceedsDiscountCodeLimit(amount, limit), exceeded)
    })
  }

  test('cleared discount has no limit', () => {
    assert.equal(exceedsDiscountCodeLimit(10001, undefined), false)
  })
})
