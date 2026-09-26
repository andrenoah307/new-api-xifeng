import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

import { createInstance } from 'i18next'

import { formatDiscountCodePendingDuration } from './pending-duration'

test('pending timeout displays exact hours, minutes or seconds', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: {
      en: {
        translation: {
          '{{count}} hour_one': '{{count}} hour',
          '{{count}} hour_other': '{{count}} hours',
          '{{count}} minute_one': '{{count}} minute',
          '{{count}} minute_other': '{{count}} minutes',
          '{{count}} second_one': '{{count}} second',
          '{{count}} second_other': '{{count}} seconds',
        },
      },
    },
  })
  for (const [seconds, want] of [
    [1800, '30 minutes'],
    [3600, '1 hour'],
    [7200, '2 hours'],
    [90, '90 seconds'],
    [60, '1 minute'],
    [1, '1 second'],
    [3601, '3601 seconds'],
  ]) {
    assert.equal(formatDiscountCodePendingDuration(seconds, i18n.t), want)
  }
  for (const invalid of [
    undefined,
    null,
    0,
    -1,
    1.5,
    '1800',
    Number.NaN,
    Infinity,
  ]) {
    assert.equal(formatDiscountCodePendingDuration(invalid, i18n.t), null)
  }
  await i18n.changeLanguage('zh')
  i18n.addResourceBundle('zh', 'translation', {
    '{{count}} minute': '{{count}} 分钟',
  })
  assert.equal(formatDiscountCodePendingDuration(1800, i18n.t), '30 分钟')
})

test('all discount-code locales interpolate durations and cleanup messages', async () => {
  const expectedMinutes = {
    en: '30 minutes',
    zh: '30 分钟',
    'zh-TW': '30 分鐘',
    fr: '30 minutes',
    ja: '30 分',
    ru: '30 минут',
    vi: '30 phút',
  }
  for (const [language, expected] of Object.entries(expectedMinutes)) {
    const { translation } = JSON.parse(
      readFileSync(
        new URL(`../../../i18n/locales/${language}.json`, import.meta.url),
        'utf8'
      )
    )
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: false,
      resources: { [language]: { translation } },
    })
    const duration = formatDiscountCodePendingDuration(1800, i18n.t)
    assert.equal(duration, expected)
    const confirmation = i18n.t(
      'Mark pending orders for this discount code created more than {{duration}} ago as expired? This does not cancel payment.',
      { duration }
    )
    assert.ok(confirmation.includes(expected))
    for (const count of [0, 1, 2, 5]) {
      const result = i18n.t(
        'Marked {{count}} timed-out pending orders as expired',
        { count }
      )
      assert.ok(result.includes(String(count)))
      assert.ok(!result.includes('{{'))
    }
  }
})
