import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  normalizePersonalRPMItems,
  personalRPMDisplayState,
  PERSONAL_RPM_REFRESH_INTERVAL,
} from './personal-rpm.ts'

describe('personal RPM presentation contract', () => {
  test('sorts by RPM, breaks ties by model, and drops zero values', () => {
    assert.deepEqual(
      normalizePersonalRPMItems([
        { model: 'z', rpm: 2 },
        { model: 'a', rpm: 2 },
        { model: 'zero', rpm: 0 },
        { model: 'one', rpm: 1 },
      ]),
      [
        { model: 'a', rpm: 2 },
        { model: 'z', rpm: 2 },
        { model: 'one', rpm: 1 },
      ]
    )
  })

  test('distinguishes empty from unavailable and never fabricates rows', () => {
    assert.equal(personalRPMDisplayState('empty', []), 'empty')
    assert.equal(personalRPMDisplayState('available', []), 'empty')
    assert.equal(
      personalRPMDisplayState('overflow', [{ model: 'hidden', rpm: 1 }]),
      'unavailable'
    )
  })

  test('uses the locked fifteen-second refresh interval', () => {
    assert.equal(PERSONAL_RPM_REFRESH_INTERVAL, 15_000)
  })
})
