import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { ticketAdminSearchSchema } from './ticket-admin-search'

const parseCases = [
  {
    name: 'keeps optional fields absent for an empty search',
    search: {},
    expected: {},
  },
  {
    name: 'passes valid search values through unchanged',
    search: {
      page: 3,
      pageSize: 25,
      keyword: 'invoice',
      scope: 'mine',
      status: 'pending',
      type: 'invoice',
    },
    expected: {
      page: 3,
      pageSize: 25,
      keyword: 'invoice',
      scope: 'mine',
      status: 'pending',
      type: 'invoice',
    },
  },
  {
    name: 'falls back to undefined for an invalid scope',
    search: { scope: 'team' },
    expected: { scope: undefined },
  },
  {
    name: 'falls back to pagination defaults for invalid types',
    search: { page: '3', pageSize: null },
    expected: { page: 1, pageSize: 10 },
  },
  {
    name: 'preserves an empty keyword',
    search: { keyword: '' },
    expected: { keyword: '' },
  },
] as const

describe('ticketAdminSearchSchema', () => {
  for (const parseCase of parseCases) {
    test(parseCase.name, () => {
      assert.deepEqual(
        ticketAdminSearchSchema.parse(parseCase.search),
        parseCase.expected
      )
    })
  }
})
