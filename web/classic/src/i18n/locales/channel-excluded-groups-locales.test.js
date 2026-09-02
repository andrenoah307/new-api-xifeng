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
import { test } from 'node:test';

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-CN', 'zh-TW'];
const requiredKeys = [
  '排除分组',
  '留空表示不排除任何分组',
  '这些用户分组的调用者永远不会被路由到本渠道，无论他们通过哪个分组调用。当该用户分组的售价低于本渠道成本时使用。',
  '请在系统设置页面编辑分组倍率以添加新的分组：',
];

for (const locale of locales) {
  test(`${locale} has complete excluded user group copy`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8'),
    );

    for (const key of requiredKeys) {
      const value = document.translation?.[key];
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`);
      assert.notEqual(value.trim(), '');
    }
  });
}
