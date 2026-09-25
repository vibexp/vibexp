import { sanitizeAdvanced } from '@/pages/admin/filters/advancedFilterParams'
import type { AdminProjectListParams } from '@/services/adminService'

import type { ProjectListContext } from '../projectListParams'
import {
  buildProjectListParams,
  LAST_RESOURCE_CREATED,
  PROJECT_ADVANCED_FILTERS,
  PROJECT_FILTERS_EXHAUSTIVE,
  PROJECT_RESOURCE_BREAKDOWN,
  PROJECT_RESOURCE_RANGES,
} from '../projectListParams'

/** The page passes the hook's cleaned `advancedParams`; derive them the same way. */
const advancedOf = (filters: Record<string, string>) =>
  sanitizeAdvanced(PROJECT_ADVANCED_FILTERS, filters).params

const CTX: Omit<ProjectListContext, 'advanced'> = {
  page: 2,
  limit: 20,
  sortBy: 'created_at',
  sortOrder: 'desc',
}

const build = (filters: Record<string, string>) =>
  buildProjectListParams(filters, { ...CTX, advanced: advancedOf(filters) })

/** Only the params that are actually present (not `undefined`). */
const present = (params: AdminProjectListParams) =>
  Object.fromEntries(
    Object.entries(params as Record<string, unknown>).filter(
      ([, value]) => value !== undefined
    )
  )

const BASE = { page: 2, limit: 20, sort_by: 'created_at', sort_order: 'desc' }

const RANGE_NAMES = PROJECT_RESOURCE_RANGES.map(filter => filter.name)

it('sends only the base params when nothing is filtered', () => {
  expect(present(build({ search: '', team_id: '', owner_email: '' }))).toEqual(
    BASE
  )
})

it('derives the hook declaration from the same filter lists as the panel', () => {
  // Exhaustiveness against the generated type is enforced at compile time
  // (`PROJECT_FILTERS_EXHAUSTIVE`); this pins that the hook owns exactly those keys.
  expect(PROJECT_FILTERS_EXHAUSTIVE).toBe(true)
  expect(PROJECT_ADVANCED_FILTERS.ranges).toEqual(RANGE_NAMES)
  expect(PROJECT_ADVANCED_FILTERS.dateRanges).toEqual([LAST_RESOURCE_CREATED])
})

it('keeps the breakdown in step with the per-type ranges', () => {
  // Same types, same order: the panel and the Resources tooltip must not drift.
  expect(PROJECT_RESOURCE_BREAKDOWN.map(type => type.label)).toEqual(
    PROJECT_RESOURCE_RANGES.filter(
      range => range.name !== 'total_resource_count'
    ).map(range => range.label)
  )
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

describe('the last-resource-created range', () => {
  it('sends local-day instants, the upper bound at end of day', () => {
    const params = build({
      last_resource_created_from: '2026-01-01',
      last_resource_created_to: '2026-01-31',
    })
    const from = new Date(String(params.last_resource_created_from))
    const to = new Date(String(params.last_resource_created_to))
    expect([from.getDate(), from.getHours()]).toEqual([1, 0])
    expect([to.getDate(), to.getHours()]).toEqual([31, 23])
  })

  it('sends one bound alone', () => {
    const params = present(build({ last_resource_created_to: '2026-01-31' }))
    expect(params).toHaveProperty('last_resource_created_to')
    expect(params).not.toHaveProperty('last_resource_created_from')
  })

  it('omits empty and malformed days', () => {
    expect(present(build({ last_resource_created_from: '' }))).toEqual(BASE)
    expect(present(build({ last_resource_created_from: 'yesterday' }))).toEqual(
      BASE
    )
  })
})

describe('the creator email', () => {
  it('is trimmed', () => {
    expect(build({ owner_email: '  a@b.co ' }).owner_email).toBe('a@b.co')
  })

  it('is omitted when blank or not an address', () => {
    // The API answers a malformed owner_email with a 400.
    for (const value of ['   ', 'boss', 'john..doe@corp.com', '<a@b.co>']) {
      expect(build({ owner_email: value }).owner_email).toBeUndefined()
    }
  })
})

it('passes search and team_id through, omitting them when empty', () => {
  expect(build({ search: 'plat', team_id: 't1' })).toMatchObject({
    search: 'plat',
    team_id: 't1',
  })
  expect(build({ search: '', team_id: '' })).toMatchObject({
    search: undefined,
    team_id: undefined,
  })
})

it('combines base, range, date-range and creator filters', () => {
  const filters = {
    search: 'plat',
    team_id: 't1',
    owner_email: 'x@corp.com',
    memory_count_min: '500',
    total_resource_count_max: '1000',
  }
  const params = buildProjectListParams(filters, {
    advanced: advancedOf(filters),
    page: 1,
    limit: 20,
    createdFrom: '2026-01-01T00:00:00.000Z',
    createdTo: '2026-02-01T00:00:00.000Z',
    sortBy: 'total_resource_count',
    sortOrder: 'asc',
  })

  expect(present(params)).toEqual({
    page: 1,
    limit: 20,
    search: 'plat',
    team_id: 't1',
    owner_email: 'x@corp.com',
    created_from: '2026-01-01T00:00:00.000Z',
    created_to: '2026-02-01T00:00:00.000Z',
    sort_by: 'total_resource_count',
    sort_order: 'asc',
    memory_count_min: 500,
    total_resource_count_max: 1000,
  } satisfies AdminProjectListParams)
})
