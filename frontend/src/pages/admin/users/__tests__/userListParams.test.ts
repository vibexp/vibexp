import { sanitizeAdvanced } from '@/pages/admin/filters/advancedFilterParams'
import type { AdminUserListParams } from '@/services/adminService'

import { USER_ADVANCED_FILTERS } from '../userAdvancedFilters'
import type { UserListContext } from '../userListParams'
import { buildUserListParams, statusParam } from '../userListParams'

/** The page passes the hook's cleaned `advancedParams`; derive them the same way. */
const advancedOf = (filters: Record<string, string>) =>
  sanitizeAdvanced(USER_ADVANCED_FILTERS, filters).params

const CTX: Omit<UserListContext, 'advanced'> = {
  page: 2,
  limit: 20,
  sortBy: 'created_at',
  sortOrder: 'desc',
}

const build = (
  filters: Record<string, string>,
  ctx: Partial<UserListContext> = {}
) =>
  buildUserListParams(filters, {
    ...CTX,
    advanced: advancedOf(filters),
    ...ctx,
  })

/** Only the params that are actually present (not `undefined`). */
const present = (params: AdminUserListParams) =>
  Object.fromEntries(
    Object.entries(params as Record<string, unknown>).filter(
      ([, value]) => value !== undefined
    )
  )

const BASE = { page: 2, limit: 20, sort_by: 'created_at', sort_order: 'desc' }

it('sends only the base params when nothing is filtered', () => {
  expect(
    present(build({ search: '', status: 'all', idp_provider: '' }))
  ).toEqual(BASE)
})

it('sends search, status, provider, created bounds and sort when set', () => {
  expect(
    present(
      build(
        { search: 'ada', status: 'suspended', idp_provider: 'github' },
        {
          createdFrom: '2026-07-01T00:00:00.000Z',
          createdTo: '2026-07-24T23:59:59.999Z',
          sortBy: 'team_count',
          sortOrder: 'asc',
        }
      )
    )
  ).toEqual({
    page: 2,
    limit: 20,
    search: 'ada',
    status: 'suspended',
    idp_provider: 'github',
    created_from: '2026-07-01T00:00:00.000Z',
    created_to: '2026-07-24T23:59:59.999Z',
    sort_by: 'team_count',
    sort_order: 'asc',
  })
})

it('spreads the cleaned advanced params and drops invalid ones', () => {
  expect(
    present(
      build({
        prompt_count_min: '3',
        team_count_max: 'abc',
        // URL days become RFC 3339 instants (local time zone).
        last_resource_created_from: '2026-07-01',
      })
    )
  ).toEqual({
    ...BASE,
    prompt_count_min: 3,
    last_resource_created_from: expect.any(String),
  })
})

describe('statusParam', () => {
  it.each([
    ['active', 'active'],
    ['suspended', 'suspended'],
    ['all', undefined],
    ['', undefined],
    ['bogus', undefined],
  ])('maps %j to %j', (input, expected) => {
    expect(statusParam(input)).toBe(expected)
  })
})
