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
const sourceFile =
  '../../features/system-settings/security/request-blacklist-section.tsx'
const requiredKeys = [
  'Saved, but failed to reload normalized settings',
  'Request Content and Domain Blacklist',
  'Inspect request text for configured keywords and domain names before relay.',
  'Off',
  'Observe',
  'Enforce',
  'Request blacklist inspection is disabled.',
  'Observe only records audit logs and allows requests.',
  'Matching requests are recorded and blocked before relay.',
  'Block status code',
  '403 (default)',
  'Matches this policy: the request is rejected and clients should not retry.',
  'Marks the request content as invalid; some SDKs surface it directly as a parameter error.',
  'Client retry mechanisms will resend the same blacklisted content and waste compute.',
  'Enabled groups',
  'All groups',
  'Leave empty to apply to all groups.',
  'Blocked content keywords',
  'Enter one content keyword per line.',
  'Blocked domains',
  'example.com',
  'Enter one hostname per line without http://, paths, or wildcards. example.com matches itself and any subdomain, but not notexample.com or example.com.evil.com.',
  'Block message',
  'This message is returned with the request ID. Do not include domains, URLs, IP addresses, or keys, or saving will be rejected.',
  'Coverage and limitations',
  'Only request text is scanned. Domain rules apply only when a domain appears as text in the prompt.',
  'Not covered: structured URL fields such as image_url, file, and video_url; Midjourney and video or music task prompts; /v1/alpha/search; Realtime voice sessions; transcription prompts; uploaded file contents; and base64.',
  'This cannot stop short links, redirects, encoding obfuscation, OCR, or targets generated autonomously by an upstream model.',
  'Unicode domains must be entered as punycode. 例え.jp is saved as xn--r8jz45g.jp; prompt text 例え.jp will not match, while xn--r8jz45g.jp will.',
  'Real outbound authorization must be enforced where HTTP requests are made, at the egress or tool layer. This feature is not complete penetration protection.',
  'Save request blacklist settings',
]

const localeDocuments = Object.fromEntries(
  locales.map((locale) => [
    locale,
    JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation: Record<string, unknown> },
  ])
)

for (const locale of locales) {
  test(`${locale} has complete request blacklist copy`, () => {
    const translations = localeDocuments[locale].translation
    for (const key of requiredKeys) {
      const value = translations[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      assert.notEqual((value as string).trim(), '')
    }
  })
}

test('request blacklist locale key order is identical', () => {
  const expected = Object.keys(localeDocuments.en.translation)
  for (const locale of locales.slice(1)) {
    assert.deepEqual(Object.keys(localeDocuments[locale].translation), expected)
  }
})

test('request blacklist source keys exist in every locale', () => {
  const source = readFileSync(new URL(sourceFile, import.meta.url), 'utf8')
  const sourceKeys = new Set<string>()
  const literalTranslationCall =
    /\bt\(\s*(?:'((?:\\.|[^'\\])*)'|"((?:\\.|[^"\\])*)")/g

  for (const match of source.matchAll(literalTranslationCall)) {
    sourceKeys.add(match[1] ?? match[2])
  }

  const missing: string[] = []
  for (const locale of locales) {
    for (const key of sourceKeys) {
      const value = localeDocuments[locale].translation[key]
      if (typeof value !== 'string' || value.trim() === '') {
        missing.push(`${locale}: ${key}`)
      }
    }
  }
  assert.deepEqual(missing, [])
})
