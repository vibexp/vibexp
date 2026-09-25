/**
 * useAdminListFilters' advanced-filter extension (#1132): URL round-trip,
 * cleaning of hand-edited values, Clear, and `hasActiveFilters`.
 */
import { act, renderHook } from '@testing-library/react'
import { endOfDay, startOfDay } from 'date-fns'
import type { ReactNode } from 'react'
import { MemoryRouter, useLocation } from 'react-router'

import type { AdvancedFilterSpec } from '../filters/advancedFilterParams'
import { useAdminListFilters } from '../useAdminListFilters'

const DEFAULTS = {
  page: '1',
  search: '',
  created_from: '',
  created_to: '',
  sort_by: 'created_at',
  sort_order: 'desc',
  status: 'all',
}

const ADVANCED: AdvancedFilterSpec = {
  ranges: ['prompt_count'],
  dateRanges: ['last_resource_created'],
  triStates: ['has_projects'],
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

function renderFilters(initialEntry = '/admin/users') {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[initialEntry]}>
      {children}
      <LocationProbe />
    </MemoryRouter>
  )
  return renderHook(
    () =>
      useAdminListFilters({
        defaults: DEFAULTS,
        sortableKeys: ['created_at'] as const,
        defaultSort: 'created_at',
        filterKeys: ['status'],
        advanced: ADVANCED,
      }),
    { wrapper }
  )
}

const params = () => new URLSearchParams(currentSearch)

describe('useAdminListFilters — advanced filters', () => {
  beforeEach(() => {
    currentSearch = ''
  })

  it('reads valid advanced params from the URL into typed values', () => {
    const { result } = renderFilters(
      '/admin/users?prompt_count_min=2&prompt_count_max=9&has_projects=true&last_resource_created_from=2026-07-01'
    )
    expect(result.current.advancedParams).toEqual({
      prompt_count_min: 2,
      prompt_count_max: 9,
      has_projects: true,
      last_resource_created_from: startOfDay(
        new Date(2026, 6, 1)
      ).toISOString(),
    })
    expect(result.current.advancedActiveCount).toBe(3)
    expect(result.current.getRange('prompt_count')).toEqual({ min: 2, max: 9 })
    expect(result.current.getTriState('has_projects')).toBe(true)
    expect(result.current.getDateRange('last_resource_created')).toEqual({
      from: new Date(2026, 6, 1),
      to: undefined,
    })
    expect(result.current.hasActiveFilters).toBe(true)
  })

  it('drops invalid URL values instead of sending them', () => {
    const { result } = renderFilters(
      '/admin/users?prompt_count_min=7&prompt_count_max=3&has_projects=maybe&last_resource_created_to=2026-13-01'
    )
    expect(result.current.advancedParams).toEqual({})
    expect(result.current.advancedActiveCount).toBe(0)
    expect(result.current.getRange('prompt_count')).toEqual({})
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('writes each advanced filter to the URL and resets the page', () => {
    const { result } = renderFilters('/admin/users?page=4')

    act(() => {
      result.current.setRange('prompt_count', { min: 1 })
    })
    expect(params().get('prompt_count_min')).toBe('1')
    expect(params().has('prompt_count_max')).toBe(false)
    expect(params().has('page')).toBe(false)

    act(() => {
      result.current.setTriState('has_projects', false)
    })
    expect(params().get('has_projects')).toBe('false')

    act(() => {
      result.current.setDateRange('last_resource_created', {
        from: new Date(2026, 6, 1),
        to: new Date(2026, 6, 3),
      })
    })
    expect(params().get('last_resource_created_from')).toBe('2026-07-01')
    expect(params().get('last_resource_created_to')).toBe('2026-07-03')
    expect(result.current.advancedParams.last_resource_created_to).toBe(
      endOfDay(new Date(2026, 6, 3)).toISOString()
    )
    expect(result.current.advancedActiveCount).toBe(3)

    act(() => {
      result.current.setTriState('has_projects', undefined)
    })
    expect(params().has('has_projects')).toBe(false)
  })

  it('counts an advanced-only filter as active', () => {
    const { result } = renderFilters()
    expect(result.current.hasActiveFilters).toBe(false)
    act(() => {
      result.current.setTriState('has_projects', true)
    })
    expect(result.current.hasActiveFilters).toBe(true)
  })

  it('Clear removes every advanced key and keeps unrelated params', () => {
    const { result } = renderFilters(
      '/admin/users?prompt_count_min=1&has_projects=true&last_resource_created_from=2026-07-01&status=active&other=keep'
    )
    act(() => {
      result.current.handleClear()
    })
    expect(currentSearch).toBe('?other=keep')
    expect(result.current.advancedActiveCount).toBe(0)
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('behaves as before when no advanced filters are declared', () => {
    const { result } = renderHook(
      () =>
        useAdminListFilters({
          defaults: DEFAULTS,
          sortableKeys: ['created_at'] as const,
          defaultSort: 'created_at',
          filterKeys: ['status'],
        }),
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <MemoryRouter initialEntries={['/admin/users?prompt_count_min=1']}>
            {children}
            <LocationProbe />
          </MemoryRouter>
        ),
      }
    )
    expect(result.current.advancedParams).toEqual({})
    expect(result.current.hasActiveFilters).toBe(false)
    act(() => {
      result.current.handleClear()
    })
    expect(params().get('prompt_count_min')).toBe('1')
  })
})
