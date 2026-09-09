import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-CN', 'zh-TW'];
const requiredKeys = [
  'Upstream Request ID Source',
  'Upstream gateway own request ID (X-Oneapi-Request-Id)',
  'Generic request ID (X-Request-Id), origin unknown, may be forwarded by a deeper proxy',
  'Unknown (recorded before source tracking)',
];

for (const locale of locales) {
  test(`${locale} has upstream request ID source copy`, () => {
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
