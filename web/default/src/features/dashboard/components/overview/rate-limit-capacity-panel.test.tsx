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

import {
  environmentManager,
  focusManager,
  QueryClient,
  QueryClientProvider,
  QueryObserver,
} from '@tanstack/react-query'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import type {
  RateLimitCapacityItem,
  RateLimitCapacityPersonal,
  RateLimitCapacityResponse,
  RateLimitCapacitySection,
} from '@/features/dashboard/types'
import { useAuthStore } from '@/stores/auth-store'

import * as capacityPanelModule from './rate-limit-capacity-panel'

const bunTestModule: string = 'bun:test'
const { jest } = await import(bunTestModule)

const { GroupTotalCapacitySection, RateLimitCapacityPanel } = capacityPanelModule
const topQueryKey = [
  'dashboard',
  'rate-limit-capacity',
  'anonymous:',
  'top',
]
const allQueryKey = [
  'dashboard',
  'rate-limit-capacity',
  'anonymous:',
  'all',
]

type RefetchCapacityQueries = (
  anyExpanded: boolean,
  refetchTop: () => Promise<unknown>,
  refetchAll: () => Promise<unknown>
) => Promise<void>

function getRefetchCapacityQueries(): RefetchCapacityQueries {
  const candidate = (
    capacityPanelModule as typeof capacityPanelModule & {
      refetchCapacityQueries?: RefetchCapacityQueries
    }
  ).refetchCapacityQueries
  assert.equal(typeof candidate, 'function')
  return candidate
}

function response(
  personal?: RateLimitCapacityPersonal
): RateLimitCapacityResponse {
  return {
    scope: 'top',
    observed_at: '2026-08-18T00:00:00Z',
    model_name_rpm_version: 1,
    group_rate_limit_version: 1,
    degraded: false,
    instance_only: false,
    backend_scope: 'redis',
    site: null,
    personal,
    total: personal?.total ?? 0,
  }
}

function groupTotalItem(
  group: string,
  overrides: Partial<RateLimitCapacityItem> = {}
): RateLimitCapacityItem {
  return {
    model: '',
    group,
    current: 12,
    limit: 30,
    unlimited: false,
    utilization: 0.4,
    over_limit: false,
    available: true,
    ...overrides,
  }
}

function groupTotalsResponse(
  items: RateLimitCapacityItem[],
  total = items.length
): RateLimitCapacityResponse {
  return {
    ...response(),
    site: {
      global: { items: [], total: 0 },
      groups: { groups: [], total: 0 },
      group_totals: { items, total },
    },
    total,
  }
}

async function renderPanelResult(
  data: RateLimitCapacityResponse,
  existingQueryClient?: QueryClient
): Promise<{ markup: string; queryClient: QueryClient }> {
  useAuthStore.getState().auth.setUser(null)
  const queryClient =
    existingQueryClient ??
    new QueryClient({
      defaultOptions: {
        queries: {
          refetchOnWindowFocus: true,
          retry: false,
        },
      },
    })
  if (!existingQueryClient) {
    queryClient.setQueryData(topQueryKey, data)
  }
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })

  const markup = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <RateLimitCapacityPanel />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return { markup, queryClient }
}

async function renderPanel(data: RateLimitCapacityResponse): Promise<string> {
  return (await renderPanelResult(data)).markup
}

async function renderGroupTotals(
  section: RateLimitCapacitySection,
  expanded: boolean
): Promise<string> {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <GroupTotalCapacitySection
        section={section}
        expanded={expanded}
        loading={false}
        onIntent={() => {}}
        onToggle={() => {}}
      />
    </I18nextProvider>
  )
}

async function capacityQueryOptions() {
  const { queryClient } = await renderPanelResult(response())
  const topQuery = queryClient.getQueryCache().find({ queryKey: topQueryKey })
  const allQuery = queryClient.getQueryCache().find({ queryKey: allQueryKey })
  assert.ok(topQuery)
  assert.ok(allQuery)
  return { top: topQuery.options, all: allQuery.options }
}

describe('personal rate-limit capacity section', () => {
  test('renders zero usage as current over limit with utilization', async () => {
    const markup = await renderPanel(
      response({
        status: 'ok',
        window_seconds: 60,
        observed_at: '2026-08-18T00:00:00Z',
        instance_only: false,
        total: 1,
        items: [
          {
            model: 'gpt-zero',
            current: 0,
            limit: 20,
            utilization: 0,
            available: true,
            unlimited: false,
            over_limit: false,
          },
        ],
      })
    )

    assert.match(markup, /My model RPM/)
    assert.match(markup, /gpt-zero/)
    assert.match(markup, /0 \/ 20/)
    assert.match(markup, /0\.0%/)
    assert.doesNotMatch(markup, /No request data yet/)
  })

  test('keeps showing the counted usage when the limit is unlimited', async () => {
    const markup = await renderPanel(
      response({
        status: 'ok',
        window_seconds: 60,
        observed_at: '2026-08-18T00:00:00Z',
        instance_only: false,
        total: 1,
        items: [
          {
            model: 'gpt-unlimited',
            current: 7,
            limit: 0,
            utilization: null,
            available: true,
            unlimited: true,
            over_limit: false,
          },
        ],
      })
    )

    assert.match(markup, /gpt-unlimited/)
    assert.match(markup, /7 \/ Unlimited/)
    assert.doesNotMatch(markup, /7 \/ 0/)
  })

  test('renders a bare unlimited label when the counter is unreadable', async () => {
    const markup = await renderPanel(
      response({
        status: 'ok',
        window_seconds: 60,
        observed_at: '2026-08-18T00:00:00Z',
        instance_only: false,
        total: 1,
        items: [
          {
            model: 'gpt-unlimited',
            current: null,
            limit: 0,
            utilization: null,
            available: true,
            unlimited: true,
            over_limit: false,
          },
        ],
      })
    )

    assert.match(markup, /Unlimited/)
    assert.doesNotMatch(markup, /\d+ \/ Unlimited/)
  })

  test('omits the entire section when personal data is absent', async () => {
    const markup = await renderPanel(response())
    assert.doesNotMatch(markup, /My model RPM/)
  })

  test('renders the unavailable placeholder instead of metric values', async () => {
    const markup = await renderPanel(
      response({
        status: 'unavailable',
        window_seconds: 60,
        observed_at: '2026-08-18T00:00:00Z',
        instance_only: false,
        total: 1,
        items: [
          {
            model: 'gpt-unavailable',
            current: null,
            limit: 20,
            utilization: null,
            available: false,
            unlimited: false,
            over_limit: false,
          },
        ],
      })
    )

    assert.match(markup, /My model RPM/)
    assert.match(markup, /Temporarily unavailable/)
    assert.doesNotMatch(markup, /0 \/ 20/)
  })
})

describe('group-total rate-limit capacity section', () => {
  test('renders when it is the only configured site section', async () => {
    const markup = await renderPanel(
      groupTotalsResponse([groupTotalItem('vip_2_cheap')])
    )

    assert.match(markup, /Group total RPM/)
    assert.match(markup, /vip_2_cheap · All models combined/)
    assert.match(markup, /12 \/ 30/)
    assert.doesNotMatch(markup, /title=""/)
  })

  test('shows three top rows and all rows after expansion', async () => {
    const items = ['alpha', 'beta', 'gamma', 'omega'].map((group) =>
      groupTotalItem(group)
    )
    const collapsed = await renderPanel(groupTotalsResponse(items, 4))
    assert.match(collapsed, /alpha · All models combined/)
    assert.match(collapsed, /gamma · All models combined/)
    assert.doesNotMatch(collapsed, /omega · All models combined/)
    assert.match(collapsed, /Show all 4 groups/)

    const expanded = await renderGroupTotals({ items, total: 4 }, true)
    assert.match(expanded, /omega · All models combined/)
    assert.match(expanded, /Collapse/)
  })

  test('uses the existing unavailable rendering for null counters', async () => {
    const markup = await renderPanel(
      groupTotalsResponse([
        groupTotalItem('vip_unavailable', {
          current: null,
          utilization: null,
          available: false,
        }),
      ])
    )

    assert.match(markup, /vip_unavailable · All models combined/)
    assert.match(markup, /Temporarily unavailable/)
    assert.doesNotMatch(markup, /0 \/ 30/)
  })
})

describe('rate-limit capacity refresh behavior', () => {
  test('does not automatically request again after 60 seconds', async () => {
    jest.useFakeTimers()
    const wasServer = environmentManager.isServer()
    environmentManager.setIsServer(() => false)

    const queryClient = new QueryClient()
    const options = (await capacityQueryOptions()).top
    let requestCount = 0
    const observer = new QueryObserver(queryClient, {
      ...options,
      queryKey: ['rate-limit-capacity', 'timer-test'],
      queryHash: undefined,
      queryFn: async () => {
        requestCount += 1
        return response()
      },
      enabled: true,
      retry: false,
    })
    const unsubscribe = observer.subscribe(() => {})

    try {
      await Promise.resolve()
      await Promise.resolve()
      assert.equal(requestCount, 1)

      jest.advanceTimersByTime(60_000)
      await Promise.resolve()
      await Promise.resolve()
      assert.equal(requestCount, 1)
    } finally {
      unsubscribe()
      queryClient.clear()
      environmentManager.setIsServer(() => wasServer)
      jest.useRealTimers()
    }
  })

  test('does not request again when the window regains focus', async () => {
    const wasServer = environmentManager.isServer()
    environmentManager.setIsServer(() => false)
    const optionsByScope = await capacityQueryOptions()

    try {
      for (const [scope, options] of Object.entries(optionsByScope)) {
        const queryClient = new QueryClient()
        let requestCount = 0
        const observer = new QueryObserver(queryClient, {
          ...options,
          queryKey: ['rate-limit-capacity', `${scope}-focus-test`],
          queryHash: undefined,
          queryFn: async () => {
            requestCount += 1
            return response()
          },
          enabled: true,
          refetchInterval: false,
          retry: false,
          staleTime: 0,
        })

        queryClient.mount()
        const unsubscribe = observer.subscribe(() => {})
        await Promise.resolve()
        await Promise.resolve()
        assert.equal(requestCount, 1, `${scope} initial request`)

        focusManager.setFocused(false)
        focusManager.setFocused(true)
        await Promise.resolve()
        await Promise.resolve()
        assert.equal(requestCount, 1, `${scope} focus request`)

        unsubscribe()
        queryClient.unmount()
        queryClient.clear()
      }
    } finally {
      focusManager.setFocused(undefined)
      environmentManager.setIsServer(() => wasServer)
    }
  })

  test('manual refresh requests top and also all while expanded', async () => {
    const refetchCapacityQueries = getRefetchCapacityQueries()
    const scopes: Array<'top' | 'all'> = []
    const refetchTop = async () => {
      scopes.push('top')
    }
    const refetchAll = async () => {
      scopes.push('all')
    }

    await refetchCapacityQueries(false, refetchTop, refetchAll)
    assert.deepEqual(scopes, ['top'])

    scopes.length = 0
    await refetchCapacityQueries(true, refetchTop, refetchAll)
    assert.deepEqual(scopes, ['top', 'all'])
  })

  test('two manual refreshes inside stale time both request top', async () => {
    jest.useFakeTimers()
    const refetchCapacityQueries = getRefetchCapacityQueries()
    let requestCount = 0
    const refetchTop = async () => {
      requestCount += 1
    }

    try {
      await refetchCapacityQueries(false, refetchTop, async () => {})
      jest.advanceTimersByTime(10)
      await refetchCapacityQueries(false, refetchTop, async () => {})
      assert.equal(requestCount, 2)
    } finally {
      jest.useRealTimers()
    }
  })

  test('shows a disabled loading refresh button while a request is in flight', async () => {
    const data = response()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    queryClient.setQueryData(topQueryKey, data)
    let resolveRequest: ((value: RateLimitCapacityResponse) => void) | undefined
    let requestCount = 0
    const request = new Promise<RateLimitCapacityResponse>((resolve) => {
      resolveRequest = resolve
    })
    const pendingFetch = queryClient.fetchQuery({
      queryKey: topQueryKey,
      queryFn: () => {
        requestCount += 1
        return request
      },
      staleTime: 0,
    })
    await Promise.resolve()

    const { markup } = await renderPanelResult(data, queryClient)
    const refreshButton = markup
      .match(/<button[^>]*>/g)
      ?.find((tag) => tag.includes('aria-label="Refresh"'))
    assert.ok(refreshButton)
    assert.match(refreshButton, / disabled(?:="")?/)
    assert.match(markup, /Refreshing\.\.\./)
    assert.equal(requestCount, 1)

    assert.ok(resolveRequest)
    resolveRequest(data)
    await pendingFetch
  })

  test('replaces the automatic-refresh copy with manual-refresh guidance', async () => {
    const markup = await renderPanel(response())
    assert.doesNotMatch(markup, /Data refreshes every 15 seconds/)
    assert.match(markup, /Click refresh for the latest data/)
    assert.match(markup, /aria-label="Refresh"/)
  })
})
