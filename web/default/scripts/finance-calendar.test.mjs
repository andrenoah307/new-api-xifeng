import { test } from 'node:test'
import assert from 'node:assert/strict'
import { financePeriod as defaultPeriod } from '../src/features/channel-finance/query-range.ts'
import { financePeriod as classicPeriod } from '../../classic/src/pages/ChannelFinance/query-range.js'

const cases = [
  ['half_month', '2024-03-01T04:00:00+08:00', '2024-02-16', '2024-03-01'],
  ['half_month', '2025-03-01T04:00:00+08:00', '2025-02-16', '2025-03-01'],
  ['half_month', '2026-01-01T04:00:00+08:00', '2025-12-16', '2026-01-01'],
  ['half_month', '2026-11-15T23:59:59+08:00', '2026-10-16', '2026-11-01'],
  ['half_month', '2026-11-16T00:00:00+08:00', '2026-11-01', '2026-11-16'],
  ['week', '2026-11-16T04:00:00+08:00', '2026-11-09', '2026-11-16'],
  ['month', '2024-03-01T04:00:00+08:00', '2024-02-01', '2024-03-01'],
  ['quarter', '2026-01-01T04:00:00+08:00', '2025-10-01', '2026-01-01'],
]
for (const [theme, period] of [['default', defaultPeriod], ['classic', classicPeriod]]) {
  test(`${theme}: calendar windows use complete UTC+8 periods`, () => {
    for (const [key, at, start, end] of cases) {
      const result = period(key, Date.parse(at))
      assert.equal(result.start, Date.parse(`${start}T00:00:00+08:00`) / 1000)
      assert.equal(result.end, Date.parse(`${end}T00:00:00+08:00`) / 1000)
    }
  })
}
