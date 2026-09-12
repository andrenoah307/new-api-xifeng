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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-TW']

// 注册表单 + 钱包自助卡片 + 系统设置「邀请码」小节共用的文案。
const requiredKeys = [
  'Invitation code',
  'Please enter your invitation code',
  'Invitation Codes',
  'Generate your own registration link. The number of invitations is controlled by the administrator policy.',
  'Generate invitation code',
  'Remaining generations',
  'Default max uses',
  'Default validity',
  '{{days}} days',
  'No invitation codes yet',
  'Copy invitation code',
  'Copy invitation link',
  'Invitation code created',
  'Used up',
  'Require invitation code for password registration',
  'Users registering with a username and password must provide a valid invitation code',
  'Require invitation code for OAuth / WeChat registration',
  'Applies only when a third-party sign-in creates a new account',
  'Allow regular users to generate invitation codes',
  'Shows the self-service invitation code card on the wallet page',
  'Invitation code policy (JSON)',
  'Configure the minimum account age, default generation quota, default max uses and validity, plus per-group and per-role overrides',
  // 微信绑定弹窗（与邀请码同批修复）
  'WeChat account bound successfully!',
  'Failed to bind WeChat account',
]

for (const locale of locales) {
  test(`${locale} has complete invitation code copy`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, string> }

    for (const key of requiredKeys) {
      const value = document.translation?.[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      assert.notEqual(String(value).trim(), '')
    }
  })
}

test('{{days}} interpolation placeholder survives every translation', () => {
  for (const locale of locales) {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, string> }
    const value = document.translation?.['{{days}} days'] ?? ''
    assert.ok(value.includes('{{days}}'), `${locale} lost the {{days}} slot`)
  }
})
