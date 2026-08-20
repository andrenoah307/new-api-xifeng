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
import { existsSync, readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const loginSource = readFileSync(
  new URL('./LoginForm.jsx', import.meta.url),
  'utf8',
);
const registerSource = readFileSync(
  new URL('./RegisterForm.jsx', import.meta.url),
  'utf8',
);
const shakeModuleUrl = new URL('./legal-consent-shake.js', import.meta.url);
const shakeSource = existsSync(shakeModuleUrl)
  ? readFileSync(shakeModuleUrl, 'utf8')
  : '';
const globalCss = readFileSync(
  new URL('../../index.css', import.meta.url),
  'utf8',
);

function findButton(source, label) {
  const button = [...source.matchAll(/<Button\b[\s\S]*?<\/Button>/g)]
    .map((match) => match[0])
    .find((candidate) => candidate.includes(label));
  assert.ok(button, `missing button containing ${label}`);
  return button;
}

describe('Classic auth consent action contracts', () => {
  test('keeps the login submit button clickable before consent', () => {
    const button = findButton(loginSource, 'onClick={handleSubmit}');

    assert.match(button, /htmlType='submit'/);
    assert.doesNotMatch(button, /disabled=.*allAgreedToTerms/s);
  });

  test('keeps the registration submit button clickable before consent', () => {
    const button = findButton(registerSource, "t('注册')");

    assert.match(button, /htmlType='submit'/);
    assert.doesNotMatch(button, /disabled=.*allAgreedToTerms/s);
  });
});

test('Classic consent shake class and global CSS definition stay in sync', () => {
  const implementationSource = `${loginSource}\n${registerSource}\n${shakeSource}`;
  const sourceClasses = new Set(
    implementationSource.match(/\blegal-consent-shake\b/g) ?? [],
  );
  const cssClasses = new Set(
    [...globalCss.matchAll(/\.(legal-consent-shake)\s*\{/g)].map(
      (match) => match[1],
    ),
  );

  assert.deepEqual([...sourceClasses], ['legal-consent-shake']);
  assert.deepEqual([...cssClasses], [...sourceClasses]);
  assert.match(globalCss, /@keyframes legal-consent-shake/);
  assert.match(
    globalCss,
    /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.legal-consent-shake[\s\S]*?animation: none !important/,
  );
});

test('Classic consent invalid styling uses real Semi danger CSS rules', () => {
  assert.match(shakeSource, /\blegal-consent-invalid\b/);
  assert.doesNotMatch(
    shakeSource,
    /(?:^|\s)(?:!?text-red-|!?border-red-|hover:!?text-red-)/,
  );
  assert.match(
    globalCss,
    /\.legal-consent-invalid \.semi-typography\s*\{[^}]*color:\s*var\(--semi-color-danger\)\s*!important;/,
  );
  assert.match(
    globalCss,
    /\.legal-consent-invalid \.semi-typography a\s*\{[^}]*color:\s*var\(--semi-color-danger\)\s*!important;/,
  );
  assert.match(
    globalCss,
    /\.legal-consent-invalid \.semi-checkbox-inner-display\s*\{[^}]*box-shadow:\s*inset 0 0 0 1px var\(--semi-color-danger\)\s*!important;/,
  );
});
