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
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { financePeriod as defaultPeriod, financeTime as defaultTime } from '../default/src/features/channel-finance/query-range.ts'
import { financePeriod as classicPeriod, financeTime as classicTime } from '../classic/src/pages/ChannelFinance/query-range.js'

for (const [name, period, time] of [['default',defaultPeriod,defaultTime],['classic',classicPeriod,classicTime]]) {
  describe(name + ' revenue calendar ranges', () => {
    for (const [kind, now, start, end] of [
      ['day', '2024-03-01T00:01:00+08:00','2024-02-29T00:00:00','2024-03-01T00:00:00'],
      ['week','2024-01-01T04:00:00+08:00','2023-12-25T00:00:00','2024-01-01T00:00:00'],
      ['month','2024-03-01T04:00:00+08:00','2024-02-01T00:00:00','2024-03-01T00:00:00'],
      ['quarter','2024-01-01T04:00:00+08:00','2023-10-01T00:00:00','2024-01-01T00:00:00'],
      ['year','2025-11-01T04:00:00+08:00','2024-01-01T00:00:00','2025-01-01T00:00:00'],
      ['recent','2026-11-01T04:17:35+08:00','2026-10-31T04:00:00','2026-11-01T04:00:00'],
    ]) {
      it(kind + ' uses closed calendar periods in UTC+8', () => {
        const result=period(kind,Date.parse(now))
        assert.equal(time(result.start),start)
        assert.equal(time(result.end),end)
        assert(result.end <= Date.parse(now)/1000)
      })
    }
    it('preserves second-precision boundaries for summary handoff',()=>{
      assert.equal(Date.parse(time(1793481237)+'+08:00')/1000,1793481237)
    })
  })
}
