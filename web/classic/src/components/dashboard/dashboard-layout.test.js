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

import { isRateLimitCapacityEnabled } from '../../helpers/rate-limit-capacity.js';

describe('Classic dashboard conditional layout', () => {
  test('renders the RPM card as a sibling before optional info panels', () => {
    const source = readFileSync(
      new URL('./index.jsx', import.meta.url),
      'utf8',
    );

    assert.match(
      source,
      /<\/div>\s*<\/div>\s*\{dashboardData\.hasRateLimitCapacityPanel && <RateLimitCapacityPanel \/>\}\s*\{\/\* 系统公告和常见问答卡片 \*\/\}\s*\{dashboardData\.hasInfoPanels && \(/,
    );
    assert.equal(source.match(/<RateLimitCapacityPanel \/>/g)?.length, 1);
    assert.doesNotMatch(
      source,
      /<div className='mb-4'>\s*<RateLimitCapacityPanel \/>/,
    );
  });

  test('keeps the RPM card enabled when all four content panels are off', () => {
    assert.equal(
      isRateLimitCapacityEnabled({
        api_info_enabled: false,
        announcements_enabled: false,
        faq_enabled: false,
        uptime_kuma_enabled: false,
        rate_limit_capacity_enabled: true,
      }),
      true,
    );
  });

  test('guards init, refresh, and manual Uptime requests with ready status', () => {
    const dashboardSource = readFileSync(
      new URL('./index.jsx', import.meta.url),
      'utf8',
    );
    const hookSource = readFileSync(
      new URL('../../hooks/dashboard/useDashboardData.js', import.meta.url),
      'utf8',
    );
    const panelSource = readFileSync(
      new URL('./UptimePanel.jsx', import.meta.url),
      'utf8',
    );

    assert.match(
      dashboardSource,
      /if \(dashboardData\.uptimeRequestEnabled\) \{\s*dashboardData\.loadUptimeData\(\);\s*\}/,
    );
    assert.match(
      hookSource,
      /if \(uptimeRequestEnabled\) \{\s*await loadUptimeData\(\);\s*\}/,
    );
    assert.match(hookSource, /requestUptimeStatus\(/);
    assert.match(panelSource, /onClick=\{loadUptimeData\}/);
  });
});
