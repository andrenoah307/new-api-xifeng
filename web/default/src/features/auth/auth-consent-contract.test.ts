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
import { describe, test } from 'node:test'

const signInSource = readFileSync(
  new URL('./sign-in/components/user-auth-form.tsx', import.meta.url),
  'utf8'
)
const signUpSource = readFileSync(
  new URL('./sign-up/components/sign-up-form.tsx', import.meta.url),
  'utf8'
)
const oauthSource = readFileSync(
  new URL('./components/oauth-providers.tsx', import.meta.url),
  'utf8'
)
const consentSource = readFileSync(
  new URL('./components/legal-consent.tsx', import.meta.url),
  'utf8'
)
const consentRowSource = readFileSync(
  new URL('./components/legal-consent-row.tsx', import.meta.url),
  'utf8'
)
const globalCss = readFileSync(
  new URL('../../styles/index.css', import.meta.url),
  'utf8'
)

function findButton(source: string, label: string): string {
  const button = [...source.matchAll(/<Button\b[\s\S]*?<\/Button>/g)]
    .map((match) => match[0])
    .find((candidate) => candidate.includes(label))
  assert.ok(button, `missing button containing ${label}`)
  return button
}

function findOAuthProviders(source: string): string {
  const block = source.match(/<OAuthProviders\b[\s\S]*?\/>/)?.[0]
  assert.ok(block, 'missing OAuthProviders block')
  return block
}

describe('Default auth consent action contracts', () => {
  test('keeps the sign-in submit button clickable before consent', () => {
    const button = findButton(signInSource, "t('Sign in')")

    assert.match(button, /disabled=\{isLoading\}/)
    assert.doesNotMatch(button, /allLegalAgreed/)
  })

  test('keeps the sign-up submit button clickable before consent while preserving Turnstile readiness', () => {
    const button = findButton(signUpSource, "t('Create account')")

    assert.match(button, /disabled=\{isLoading \|\| !turnstileReady\}/)
    assert.doesNotMatch(button, /allLegalAgreed/)
  })

  test('removes consent from every action disabled expression', () => {
    const disabledExpressions = `${signInSource}\n${signUpSource}`.match(
      /disabled=\{[\s\S]*?\}/g
    )

    assert.ok(disabledExpressions)
    assert.equal(
      disabledExpressions.some((expression) =>
        expression.includes('allLegalAgreed')
      ),
      false
    )
  })

  test('routes sign-in and sign-up OAuth actions through consent preflight', () => {
    for (const source of [signInSource, signUpSource]) {
      const block = findOAuthProviders(source)
      assert.match(block, /disabled=\{isLoading\}/)
      assert.match(block, /onBeforeAction=\{validateLegalConsent\}/)
    }
    assert.match(oauthSource, /onBeforeAction\?: \(\) => boolean/)
    assert.match(oauthSource, /runOAuthProviderAction/)
  })
})

test('Default consent shake class and global CSS definition stay in sync', () => {
  const sourceClasses = new Set(
    `${consentSource}\n${consentRowSource}`.match(
      /\banimate-[a-z0-9-]*consent-shake\b/g
    ) ?? []
  )
  const cssClasses = new Set(
    [...globalCss.matchAll(/--(animate-[a-z0-9-]*consent-shake)\s*:/g)].map(
      (match) => match[1]
    )
  )

  assert.deepEqual([...sourceClasses], ['animate-consent-shake'])
  assert.deepEqual([...cssClasses], [...sourceClasses])
  assert.match(globalCss, /@keyframes consent-shake/)
  assert.match(
    globalCss,
    /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.animate-consent-shake[\s\S]*?animation: none !important/
  )
})
