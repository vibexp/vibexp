import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'

import { useResourceListFilters } from './useResourceListFilters'

const DEFAULTS = {
  page: '1',
  search: '',
  metadata: '',
  sort_order: 'desc',
  type: 'all',
}

function wrapperFor(initialEntry: string) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(
      MemoryRouter,
      { initialEntries: [initialEntry] },
      children
    )
  }
}

function renderFilters(
  initialEntry = '/list',
  overrides: { projectId?: string; isProjectLoading?: boolean } = {}
) {
  return renderHook(
    (props: { projectId?: string; isProjectLoading?: boolean }) =>
      useResourceListFilters({
        defaults: DEFAULTS,
        filterKeys: ['type'],
        projectId: props.projectId,
        isProjectLoading: props.isProjectLoading ?? false,
        debounceMs: 300,
      }),
    {
      wrapper: wrapperFor(initialEntry),
      initialProps: { isProjectLoading: false, ...overrides },
    }
  )
}

describe('useResourceListFilters', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('derives page and sort order, defaulting sensibly', () => {
    const { result } = renderFilters('/list')

    expect(result.current.page).toBe(1)
    expect(result.current.sortOrder).toBe('desc')
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('reads page and sort order from the URL', () => {
    const { result } = renderFilters('/list?page=4&sort_order=asc')

    expect(result.current.page).toBe(4)
    expect(result.current.sortOrder).toBe('asc')
  })

  it('parses the metadata param and re-serializes it canonically', () => {
    const { result } = renderFilters(
      '/list?metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D'
    )

    expect(result.current.metadata).toEqual({ env: ['prod'] })
    expect(result.current.metadataParam).toBe('{"env":["prod"]}')
    expect(result.current.hasActiveFilters).toBe(true)
  })

  it('drops a malformed metadata param rather than forwarding it', () => {
    const { result } = renderFilters('/list?metadata=not-json')

    expect(result.current.metadata).toEqual({})
    expect(result.current.metadataParam).toBeUndefined()
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('counts a domain filter key as active', () => {
    const { result } = renderFilters('/list?type=cursor')

    expect(result.current.hasActiveFilters).toBe(true)
  })

  it('debounces the search input into the URL', () => {
    const { result } = renderFilters()

    act(() => {
      result.current.setSearchInput('a')
    })
    act(() => {
      result.current.setSearchInput('ap')
    })
    expect(result.current.filters.search).toBe('')

    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(result.current.filters.search).toBe('ap')
  })

  it('clear resets the URL and the uncommitted search box', () => {
    const { result } = renderFilters('/list?search=nope&type=cursor')
    expect(result.current.searchInput).toBe('nope')

    act(() => {
      result.current.handleClear()
    })

    expect(result.current.filters.search).toBe('')
    expect(result.current.filters.type).toBe('all')
    // Stale text would re-commit on the next debounce tick and undo the clear.
    expect(result.current.searchInput).toBe('')
  })

  it('setMetadata writes the JSON param, and an empty filter clears it', () => {
    const { result } = renderFilters()

    act(() => {
      result.current.setMetadata({ env: ['prod'] })
    })
    expect(result.current.filters.metadata).toBe('{"env":["prod"]}')

    act(() => {
      result.current.setMetadata({})
    })
    expect(result.current.filters.metadata).toBe('')
  })

  it('a restoring project does not reset the page', () => {
    const { result, rerender } = renderFilters('/list?page=3', {
      isProjectLoading: true,
    })
    expect(result.current.page).toBe(3)

    // The restore completes and delivers the persisted project.
    rerender({ projectId: 'p1', isProjectLoading: false })

    expect(result.current.page).toBe(3)
  })

  it('a genuine project switch resets the page', () => {
    const { result, rerender } = renderFilters('/list?page=3', {
      isProjectLoading: false,
    })
    expect(result.current.page).toBe(3)

    rerender({ projectId: 'p1', isProjectLoading: false })

    expect(result.current.page).toBe(1)
  })

  it('setPage writes the page without clearing other filters', () => {
    const { result } = renderFilters('/list?type=cursor')

    act(() => {
      result.current.setPage(5)
    })

    expect(result.current.page).toBe(5)
    expect(result.current.filters.type).toBe('cursor')
  })
})
