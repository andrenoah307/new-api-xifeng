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
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { calculateLogProfit } from './profit.js';

test('profit uses base amounts, keeps zero cost and negative results', () => {
  assert.equal(
    calculateLogProfit(100, 2, 500000, 500000, {
      status: 'estimated',
      amount_usd: '0.25',
    }),
    0.75,
  );
  assert.equal(
    calculateLogProfit(100, 2, 500000, 500000, {
      status: 'stale',
      amount_usd: '1.25',
    }),
    -0.25,
  );
  assert.equal(
    calculateLogProfit(100, 2, 250000, 1000000, {
      status: 'estimated',
      amount_usd: '0',
    }),
    0.25,
  );
});

test('profit is unavailable for non-root roles, unpriced logs and invalid amounts', () => {
  const cost = { status: 'estimated', amount_usd: '0.25' };
  for (const role of [0, 1, 5, 10, 99, 101]) {
    assert.equal(calculateLogProfit(role, 2, 500000, 500000, cost), null);
  }
  for (const type of [0, 1, 3, 4, 5, 6]) {
    assert.equal(calculateLogProfit(100, type, 500000, 500000, cost), null);
  }
  for (const invalid of [
    undefined,
    { status: 'unknown', amount_usd: '1' },
    ...['', ' ', 'NaN', 'Infinity', '-1'].map((amount_usd) => ({
      status: 'estimated',
      amount_usd,
    })),
  ]) {
    assert.equal(calculateLogProfit(100, 2, 500000, 500000, invalid), null);
  }
  assert.equal(calculateLogProfit(100, 2, 500000, 0, cost), null);
  assert.equal(calculateLogProfit(100, 2, NaN, 500000, cost), null);
});
