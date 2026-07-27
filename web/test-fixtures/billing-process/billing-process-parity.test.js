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

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { reconstructBillingProcess as reconstructClassic } from "../../classic/src/helpers/billing-process.js";
import { reconstructBillingProcess as reconstructDefault } from "../../default/src/features/usage-logs/lib/billing-process.ts";

const fixture = JSON.parse(
  readFileSync(new URL("./production.json", import.meta.url), "utf8"),
);

test("Classic and Default return identical structures for every production fixture", () => {
  for (const entry of fixture.cases) {
    const input = {
      log: entry.log,
      other: entry.other,
      quotaPerUnit: fixture.quota_per_unit,
    };
    assert.deepStrictEqual(
      reconstructClassic(input),
      reconstructDefault(input),
      entry.id,
    );
  }
});
