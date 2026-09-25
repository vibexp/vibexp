import { sanitizeAdvanced } from '@/pages/admin/filters/advancedFilterParams'
import type { AdminTeamListParams } from '@/services/adminService'

import type { TeamListContext } from '../teamListParams'
import {
  buildTeamListParams,
  isPersonalParam,
  TEAM_ADVANCED_FILTERS,
  TEAM_FILTERS_EXHAUSTIVE,
  TEAM_MEMBERSHIP_RANGES,
  TEAM_RESOURCE_RANGES,
  TEAM_SETUP_TRISTATES,
} from '../teamListParams'

/** The page passes the hook's cleaned `advancedParams`; derive them the same way. */
const advancedOf = (filters: Record<string, string>) =>
  sanitizeAdvanced(TEAM_ADVANCED_FILTERS, filters).params

const CTX: Omit<TeamListContext, 'advanced'> = {
  page: 2,
  limit: 20,
  sortBy: 'created_at',
  sortOrder: 'desc',
}

const build = (filters: Record<string, string>) =>
  buildTeamListParams(filters, { ...CTX, advanced: advancedOf(filters) })

/** Only the params that are actually present (not `undefined`). */
const present = (params: AdminTeamListParams) =>
  Object.fromEntries(
    Object.entries(params as Record<string, unknown>).filter(
      ([, value]) => value !== undefined
    )
  )

const BASE = { page: 2, limit: 20, sort_by: 'created_at', sort_order: 'desc' }

const RANGE_NAMES = [...TEAM_MEMBERSHIP_RANGES, ...TEAM_RESOURCE_RANGES].map(
  filter => filter.name
)
const TRISTATE_NAMES = TEAM_SETUP_TRISTATES.map(filter => filter.name)

it('sends only the base params when nothing is filtered', () => {
  expect(present(build({ kind: 'all', search: '', owner_email: '' }))).toEqual(
    BASE
  )
})

it('derives the hook declaration from the same filter lists as the panel', () => {
  // Exhaustiveness against the generated type is enforced at compile time
  // (`TEAM_FILTERS_EXHAUSTIVE`); this pins that the hook owns exactly those keys.
  expect(TEAM_FILTERS_EXHAUSTIVE).toBe(true)
  expect(TEAM_ADVANCED_FILTERS.ranges).toEqual(RANGE_NAMES)
  expect(TEAM_ADVANCED_FILTERS.triStates).toEqual(TRISTATE_NAMES)
})

describe.each(RANGE_NAMES)('the %s range', name => {
  const minKey = `${name}_min`
  const maxKey = `${name}_max`

  it('sends min alone', () => {
    expect(present(build({ [minKey]: '3' }))).toEqual({ ...BASE, [minKey]: 3 })
  })

  it('sends max alone', () => {
    expect(present(build({ [maxKey]: '7' }))).toEqual({ ...BASE, [maxKey]: 7 })
  })

  it('sends both bounds', () => {
    expect(present(build({ [minKey]: '0', [maxKey]: '7' }))).toEqual({
      ...BASE,
      [minKey]: 0,
      [maxKey]: 7,
    })
  })

  it('omits empty and garbage bounds', () => {
    expect(present(build({ [minKey]: '', [maxKey]: 'abc' }))).toEqual(BASE)
    expect(present(build({ [minKey]: '-1', [maxKey]: '1.5' }))).toEqual(BASE)
  })

  it('drops a contradictory pair whole', () => {
    expect(present(build({ [minKey]: '9', [maxKey]: '2' }))).toEqual(BASE)
  })
})

describe.each(TRISTATE_NAMES)('the %s tri-state', name => {
  it('sends nothing for "any"', () => {
    expect(present(build({ [name]: '' }))).toEqual(BASE)
    expect(present(build({ [name]: 'maybe' }))).toEqual(BASE)
  })

  it('sends true for yes and false for no', () => {
    expect(build({ [name]: 'true' })).toHaveProperty(name, true)
    // `false` is a real filter, never a stand-in for "any".
    expect(build({ [name]: 'false' })).toHaveProperty(name, false)
  })
})

describe('owner_email', () => {
  // The address rules themselves are ownerEmailParam's, tested beside it in
  // filters/__tests__/advancedFilterParams.test.ts; this pins the wiring.
  it('is trimmed', () => {
    expect(build({ owner_email: '  a@b.co ' }).owner_email).toBe('a@b.co')
  })

  it('is omitted when blank or not an address', () => {
    // The API answers a malformed owner_email with a 400.
    for (const value of ['   ', 'boss', 'john..doe@corp.com']) {
      expect(build({ owner_email: value }).owner_email).toBeUndefined()
    }
  })
})

it('maps the team kind onto is_personal unchanged', () => {
  expect(isPersonalParam('personal')).toBe(true)
  expect(isPersonalParam('shared')).toBe(false)
  expect(isPersonalParam('all')).toBeUndefined()
  expect(build({ kind: 'shared' }).is_personal).toBe(false)
  expect(build({}).is_personal).toBeUndefined()
})

it('combines base, range, tri-state and owner filters', () => {
  const filters = {
    search: 'eng',
    kind: 'shared',
    owner_email: 'x@corp.com',
    member_count_min: '10',
    total_resource_count_max: '500',
    embedding_configured: 'false',
    github_configured: 'true',
  }
  const params = buildTeamListParams(filters, {
    advanced: advancedOf(filters),
    page: 1,
    limit: 20,
    createdFrom: '2026-01-01T00:00:00.000Z',
    createdTo: '2026-02-01T00:00:00.000Z',
    sortBy: 'owner_count',
    sortOrder: 'asc',
  })

  expect(present(params)).toEqual({
    page: 1,
    limit: 20,
    search: 'eng',
    is_personal: false,
    owner_email: 'x@corp.com',
    created_from: '2026-01-01T00:00:00.000Z',
    created_to: '2026-02-01T00:00:00.000Z',
    sort_by: 'owner_count',
    sort_order: 'asc',
    member_count_min: 10,
    total_resource_count_max: 500,
    embedding_configured: false,
    github_configured: true,
  } satisfies AdminTeamListParams)
})
