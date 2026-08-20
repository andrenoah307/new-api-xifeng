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
import { describe, test } from 'node:test';

const source = readFileSync(
  new URL('./RateLimitCapacityPanel.jsx', import.meta.url),
  'utf8',
);

describe('Classic RPM overview rendering contracts', () => {
  test('renders only finite available Global metrics without an unlimited branch', () => {
    const metricAvailableSource = source.match(
      /function metricAvailable\(metric\) \{[\s\S]*?\n\}/,
    )?.[0];

    assert.ok(metricAvailableSource);
    assert.doesNotMatch(metricAvailableSource, /unlimited/);
    assert.doesNotMatch(source, /metric(?:\?)?\.unlimited/);
    assert.match(
      source,
      /\{displayedGlobalItems\.map\(\(item\) => \([\s\S]*?metric=\{item\}/,
    );
  });

  test('keeps the unavailable fallback when a counter cannot be read', () => {
    assert.match(
      source,
      /let value = t\('暂时不可用'\);\s*if \(available\) \{\s*value = `\$\{formatCount\(metric\.current\)\} \/ \$\{formatCount\(metric\.limit\)\}`;\s*\}/,
    );
  });

  test('hides the card only after a successful response has no site or personal data', () => {
    const visibilityExpression = source.match(
      /if \((data && !data\.site && !data\.personal)\) return null;/,
    )?.[1];

    assert.ok(visibilityExpression);
    const shouldHide = Function(
      'data',
      `return Boolean(${visibilityExpression});`,
    );

    assert.equal(shouldHide({ site: null, personal: null, total: 0 }), true);
    assert.equal(shouldHide({ site: null, personal: null, total: 12 }), true);
    assert.equal(
      shouldHide({
        site: null,
        personal: { status: 'ok', items: [] },
        total: 0,
      }),
      false,
    );
  });

  test('does not hide loading or error states before data exists', () => {
    const visibilityExpression = source.match(
      /if \((data && !data\.site && !data\.personal)\) return null;/,
    )?.[1];

    assert.ok(visibilityExpression);
    const shouldHide = Function(
      'data',
      `return Boolean(${visibilityExpression});`,
    );

    assert.equal(shouldHide(null), false);
    assert.match(source, /\{topLoading && !topData && \(/);
    assert.match(source, /\{topError && !topData && \(/);
  });

  test('owns the dashboard bottom spacing on the rendered card', () => {
    assert.match(source, /<Card\s+className='mb-4 shadow-sm !rounded-2xl'/);
  });
});
