import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  normalizePersonalRPMItems,
  personalRPMDisplayState,
  PERSONAL_RPM_STALE_TIME,
} from './personal-rpm.ts'

describe('personal RPM presentation contract', () => {
  test('sorts available metrics by current usage and preserves zero usage', () => {
    const zero = {
      model: 'zero',
      group: '',
      current: 0,
      limit: 20,
      utilization: 0,
      available: true,
      unlimited: false,
      over_limit: false,
    }
    const z = {
      model: 'z',
      group: '',
      current: 2,
      limit: 20,
      utilization: 0.1,
      available: true,
      unlimited: false,
      over_limit: false,
    }
    const a = { ...z, model: 'a' }
    const one = { ...z, model: 'one', current: 1, utilization: 0.05 }

    assert.deepEqual(
      normalizePersonalRPMItems([z, zero, one, a]),
      [a, z, one, zero]
    )
  })

  test('keeps unavailable metrics without letting their counters affect sorting', () => {
    const available = {
      model: 'available',
      group: '',
      current: 1,
      limit: 20,
      utilization: 0.05,
      available: true,
      unlimited: false,
      over_limit: false,
    }
    const unavailableZ = {
      ...available,
      model: 'z-unavailable',
      current: 999,
      utilization: null,
      available: false,
    }
    const unavailableA = {
      ...available,
      model: 'a-unavailable',
      current: null,
      utilization: null,
      available: false,
    }

    assert.deepEqual(
      normalizePersonalRPMItems([unavailableZ, available, unavailableA]),
      [available, unavailableA, unavailableZ]
    )
    assert.deepEqual(
      normalizePersonalRPMItems([unavailableA, unavailableZ]),
      [unavailableA, unavailableZ]
    )
  })

  test('drops malformed metrics instead of fabricating capacity values', () => {
    const valid = {
      model: 'valid',
      group: '',
      current: 0,
      limit: 20,
      utilization: 0,
      available: true,
      unlimited: false,
      over_limit: false,
    }

    assert.deepEqual(normalizePersonalRPMItems(null), [])
    assert.deepEqual(
      normalizePersonalRPMItems([
        null,
        { ...valid, model: '', group: '' },
        { ...valid, group: undefined },
        { ...valid, model: null },
        { ...valid, group: null },
        { ...valid, current: -1 },
        { ...valid, limit: Number.POSITIVE_INFINITY },
        { ...valid, utilization: -0.1 },
        { ...valid, available: 'yes' },
        valid,
      ]),
      [valid]
    )
  })

  test('keeps group-level metrics and rejects rows without either identity', () => {
    const groupMetric = {
      model: '',
      group: 'vip',
      current: 3,
      limit: 20,
      utilization: 0.15,
      available: true,
      unlimited: false,
      over_limit: false,
    }

    assert.deepEqual(normalizePersonalRPMItems([groupMetric]), [groupMetric])
    assert.deepEqual(
      normalizePersonalRPMItems([{ ...groupMetric, group: '' }]),
      []
    )
  })

  test('breaks usage ties by model or group identity', () => {
    const groupMetric = {
      model: '',
      group: 'beta',
      current: 2,
      limit: 20,
      utilization: 0.1,
      available: true,
      unlimited: false,
      over_limit: false,
    }
    const modelMetric = { ...groupMetric, model: 'alpha', group: '' }

    assert.deepEqual(normalizePersonalRPMItems([groupMetric, modelMetric]), [
      modelMetric,
      groupMetric,
    ])
  })

  test('distinguishes empty from unavailable and never fabricates rows', () => {
    assert.equal(personalRPMDisplayState('empty', []), 'empty')
    assert.equal(personalRPMDisplayState('ok', []), 'empty')
    assert.equal(
      personalRPMDisplayState('unavailable', [
        {
          model: 'hidden',
          group: '',
          current: null,
          limit: 20,
          utilization: null,
          available: false,
          unlimited: false,
          over_limit: false,
        },
      ]),
      'unavailable'
    )
  })

  test('uses the locked fifteen-second stale threshold', () => {
    assert.equal(PERSONAL_RPM_STALE_TIME, 15_000)
  })
})
