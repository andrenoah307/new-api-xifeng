/*
Copyright (C) 2025 QuantumNous

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
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  deleteGroupRateLimitRule as deleteClassic,
  parseGroupRateLimitConfig as parseClassic,
  upsertGroupRateLimitRule as upsertClassic,
  validateGroupRateLimitRule as validateClassic,
} from '../../classic/src/helpers/group-rate-limit.js';
import {
  deleteGroupRateLimitRule as deleteDefault,
  parseGroupRateLimitConfig as parseDefault,
  upsertGroupRateLimitRule as upsertDefault,
  validateGroupRateLimitRule as validateDefault,
} from '../../default/src/features/system-settings/request-limits/lib/group-rate-limit.ts';

const fixture = JSON.parse(
  readFileSync(new URL('./vectors.json', import.meta.url), 'utf8'),
);

for (const entry of fixture.cases) {
  test(`Classic and Default agree for ${entry.id}`, () => {
    const parseInput = entry.rawJson;
    const validateInput = entry.rule;
    const mutationArgs = [
      entry.rawJson,
      entry.rule,
      entry.originalGroupName,
    ];

    assert.deepStrictEqual(
      parseClassic(parseInput),
      parseDefault(parseInput),
      `${entry.id}: parse`,
    );
    assert.deepStrictEqual(
      validateClassic(validateInput),
      validateDefault(validateInput),
      `${entry.id}: validate`,
    );
    assert.deepStrictEqual(
      upsertClassic(...mutationArgs),
      upsertDefault(...mutationArgs),
      `${entry.id}: upsert`,
    );
    assert.deepStrictEqual(
      deleteClassic(entry.rawJson, entry.deleteGroupName),
      deleteDefault(entry.rawJson, entry.deleteGroupName),
      `${entry.id}: delete`,
    );
  });
}
