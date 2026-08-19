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
const sourceFiles = [
  '../../features/system-settings/request-limits/model-name-rpm-visual-editor.tsx',
  '../../features/system-settings/request-limits/rate-limit-visual-editor.tsx',
  '../../features/system-settings/request-limits/rate-limit-section.tsx',
  '../../features/system-settings/request-limits/model-name-rpm-dialog.tsx',
  '../../features/dashboard/components/overview/rate-limit-capacity-panel.tsx',
]
const requiredKeys = [
  'Disabled by default. When enabled, the user dashboard displays and queries the RPM overview card.',
  'Unlimited',
  '0 means unlimited; usage is still counted and displayed.',
  'Global RPM must be an integer between 0 and 1,000,000 (0 means unlimited)',
  'When the global RPM is 0 (unlimited), configure at least one per-user or per-group limit; otherwise delete this model rule',
  'global_rpm must be an integer from 0 to 1,000,000; 0 means unlimited (usage is still counted) and then at least one user_rpm or group_rpm is required. Delete a model rule to disable it; set enabled to false to disable all rules.',
  'Group total RPM',
  'All models combined',
  'Add group total',
  'No group total RPM limits configured.',
  'Group total name is required',
  'Group total name must not exceed 64 characters',
  'Group total name must not contain whitespace or control characters',
  'This group already has a total RPM limit',
  'Total RPM',
  'Total RPM must be an integer between 1 and 1,000,000; delete the group entry to disable it',
  'Models not listed in the models section are not subject to model-specific RPM limits.',
  'Top-level group total limits apply to every model in the group, including models not listed in the models section.',
]

for (const locale of locales) {
  test(`${locale} has non-empty RPM card settings copy`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, unknown> }

    for (const key of requiredKeys) {
      const value = document.translation?.[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      assert.notEqual((value as string).trim(), '')
    }
  })
}

test('whitelisted RPM sources have complete and live locale keys', () => {
  const sourceText = sourceFiles
    .map((file) => readFileSync(new URL(file, import.meta.url), 'utf8'))
    .join('\n')
  const sourceKeys = new Set<string>()
  const literalTranslationCall =
    /\bt\(\s*(?:'((?:\\.|[^'\\])*)'|"((?:\\.|[^"\\])*)")/g

  for (const match of sourceText.matchAll(literalTranslationCall)) {
    const key = (match[1] ?? match[2])
      .replaceAll('\\n', '\n')
      .replaceAll("\\'", "'")
      .replaceAll('\\"', '"')
      .replaceAll('\\\\', '\\')
    sourceKeys.add(key)
  }

  const missingOrEmpty: string[] = []
  for (const locale of locales) {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, unknown> }
    for (const key of sourceKeys) {
      const value = document.translation?.[key]
      if (typeof value !== 'string' || value.trim() === '') {
        missingOrEmpty.push(`${locale}: ${key}`)
      }
    }
  }
  assert.deepEqual(missingOrEmpty, [])

  const unreferencedRequiredKeys = requiredKeys.filter(
    (key) =>
      !sourceText.includes(`'${key}'`) && !sourceText.includes(`"${key}"`)
  )
  assert.deepEqual(unreferencedRequiredKeys, [])
})
