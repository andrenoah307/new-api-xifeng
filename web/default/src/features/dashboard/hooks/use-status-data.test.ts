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
import { describe, test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import type { SystemStatus } from '@/features/auth/types'

import { useDashboardContentVisibility } from './use-status-data'

function VisibilityProbe() {
  const visibility = useDashboardContentVisibility()
  return createElement('output', {
    'data-api-info': visibility.apiInfo,
    'data-announcements': visibility.announcements,
    'data-faq': visibility.faq,
    'data-uptime-kuma': visibility.uptimeKuma,
    'data-rate-limit-capacity': visibility.rateLimitCapacity,
  })
}

function renderVisibility(status: SystemStatus | null): string {
  const queryClient = new QueryClient()
  if (status !== null) queryClient.setQueryData(['status'], status)

  return renderToStaticMarkup(
    createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(VisibilityProbe)
    )
  )
}

describe('dashboard content visibility', () => {
  const rateLimitCapacityCases: Array<[
    string,
    SystemStatus | null,
    boolean,
  ]> = [
    ['true', { rate_limit_capacity_enabled: true }, true],
    ['false', { rate_limit_capacity_enabled: false }, false],
    ['undefined', { rate_limit_capacity_enabled: undefined }, false],
    ['missing', {}, false],
    ['null status', null, false],
  ]

  for (const [label, status, expected] of rateLimitCapacityCases) {
    test(`shows rate-limit capacity only for ${label}`, () => {
      const markup = renderVisibility(status)
      assert.equal(
        markup.includes('data-rate-limit-capacity="true"'),
        expected
      )
    })
  }

  test('keeps the other dashboard panels opt-out', () => {
    const markup = renderVisibility({})
    assert.match(markup, /data-api-info="true"/)
    assert.match(markup, /data-announcements="true"/)
    assert.match(markup, /data-faq="true"/)
    assert.match(markup, /data-uptime-kuma="true"/)
  })
})
