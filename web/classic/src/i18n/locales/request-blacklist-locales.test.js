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
const sourceFiles = [
  '../../pages/Setting/Operation/SettingsRequestBlacklist.jsx',
  '../../pages/Setting/Operation/SettingsSensitiveWords.jsx',
];
const requiredKeys = [
  '请求内容与域名黑名单',
  '仅观察',
  '拦截',
  '请求黑名单检查已关闭。',
  '仅观察只记录审计日志并放行请求。',
  '命中后记录审计日志，并在转发前拦截请求。',
  '拦截状态码',
  '403（默认）',
  '符合本功能意图：请求被策略拒绝，客户端不应重试。',
  '表示请求内容不合规；部分 SDK 会当成参数错误直接抛出。',
  '客户端重试机制会生效，同一份黑名单内容会被反复提交并白白消耗算力。',
  '生效分组',
  '留空表示对全部分组生效。',
  '内容关键词',
  '一行一个内容关键词。',
  '一行一个内容关键词',
  '域名',
  '一行一个主机名，不带 http://、路径或通配符。example.com 同时匹配自身及任意子域，但不匹配 notexample.com 或 example.com.evil.com。',
  '拦截文案',
  '该文案会连同 request id 一起返回给调用方。不要填写域名、URL、IP 或密钥，否则保存会被拒绝。',
  '能力边界',
  '只扫描请求文本。域名黑名单只在域名以文本形式出现在 prompt 中时生效。',
  '不覆盖 image_url、file、video_url 等结构化 URL 字段，Midjourney 与视频或音乐任务的 prompt、/v1/alpha/search、Realtime 语音会话、音频转写 prompt、上传文件内容及 base64。',
  '无法拦截短链、跳转、编码混淆、OCR，以及上游模型自主生成的目标。',
  'Unicode 域名需填写 punycode。例え.jp 会保存为 xn--r8jz45g.jp；prompt 中的原文 例え.jp 不会命中，punycode 才会命中。',
  '真正的出站访问授权应在实际发起 HTTP 请求的 egress 或 tool 层完成；本功能不是完整的渗透防护。',
  '保存请求黑名单设置',
  '以下设置已生效：{{items}}。保存“{{failed}}”失败：{{message}}',
];

for (const locale of locales) {
  test(`${locale} has complete request blacklist copy`, () => {
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

test('request blacklist source keys exist in every locale', () => {
  const source = sourceFiles
    .map((file) => readFileSync(new URL(file, import.meta.url), 'utf8'))
    .join('\n');
  const sourceKeys = new Set();
  const literalTranslationCall =
    /\bt\(\s*(?:'((?:\\.|[^'\\])*)'|"((?:\\.|[^"\\])*)")/g;

  for (const match of source.matchAll(literalTranslationCall)) {
    sourceKeys.add(match[1] ?? match[2]);
  }

  const missing = [];
  for (const locale of locales) {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8'),
    );
    for (const key of sourceKeys) {
      const value = document.translation?.[key];
      if (typeof value !== 'string' || value.trim() === '') {
        missing.push(`${locale}: ${key}`);
      }
    }
  }
  assert.deepEqual(missing, []);
});
