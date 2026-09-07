import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-CN', 'zh-TW'];
const requiredKeys = [
  '下载选中',
  '请立即保存这些码，关闭本对话框后将无法再次查看。',
  '已显示 {{shown}} / {{count}}',
  '创建中途失败，出错前已生成 {{count}} 个码。',
];

for (const locale of locales) {
  test(`${locale} has complete generated codes copy`, () => {
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
