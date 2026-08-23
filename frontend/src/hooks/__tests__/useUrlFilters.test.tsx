/**
 * useUrlFilters (#456): the query-string contract the three admin list pages
 * (#459/#460/#461) build on.
 */
import { act, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router'

import type { UrlFilters } from '@/hooks/useUrlFilters'
import { useUrlFilters } from '@/hooks/useUrlFilters'

// A type alias, not an interface: `UrlFilters` requires an index signature, and
// an interface's declared members do not satisfy one.
type Filters = {
  page: string
  search: string
  status: string
  sort_by: string
} & UrlFilters

const DEFAULTS: Filters = {
  page: '1',
  search: '',
  status: 'all',
  sort_by: 'created_at',
}

interface HookResult {
  filters: Filters
  setFilters: (next: Partial<Filters>) => void
  resetFilters: () => void
}

let hook: HookResult
let goBack: () => void
let renderCount = 0
let lastFilters: Filters | undefined
let filtersIdentityChanges = 0

function Probe() {
  hook = useUrlFilters<Filters>({ ...DEFAULTS })
  const navigate = useNavigate()
  goBack = () => {
    void navigate(-1)
  }
  const { search } = useLocation()
  renderCount += 1
  if (hook.filters !== lastFilters) {
    filtersIdentityChanges += 1
    lastFilters = hook.filters
  }
  return <span data-testid="search">{search}</span>
}

function renderProbe(initialEntry = '/admin/users') {
  renderCount = 0
  lastFilters = undefined
  filtersIdentityChanges = 0
  return render(<Probe />, {
    wrapper: ({ children }: Readonly<{ children: ReactNode }>) => (
      <MemoryRouter initialEntries={[initialEntry]}>{children}</MemoryRouter>
    ),
  })
}

const urlSearch = () => screen.getByTestId('search').textContent ?? ''
const param = (name: string) => new URLSearchParams(urlSearch()).get(name)

it('falls back to the defaults when the URL carries no params', () => {
  renderProbe()

  expect(hook.filters).toEqual(DEFAULTS)
  expect(urlSearch()).toBe('')
})

it('reads applied filters out of the URL', () => {
  renderProbe('/admin/users?search=ada&status=suspended&page=3')

  expect(hook.filters).toEqual({
    page: '3',
    search: 'ada',
    status: 'suspended',
    sort_by: 'created_at',
  })
})

it('round-trips a value through the query string', () => {
  renderProbe()

  act(() => {
    hook.setFilters({ search: 'ada' })
  })

  expect(urlSearch()).toBe('?search=ada')
  expect(hook.filters.search).toBe('ada')
})

it('keeps a value equal to its default out of the URL', () => {
  renderProbe('/admin/users?status=suspended')

  act(() => {
    hook.setFilters({ status: 'all' })
  })

  // 'all' is the default, so it is removed rather than written — two routes to
  // the same view must produce the same URL.
  expect(urlSearch()).toBe('')
  expect(hook.filters.status).toBe('all')
})

it('keeps an emptied filter out of the URL', () => {
  renderProbe('/admin/users?search=ada')

  act(() => {
    hook.setFilters({ search: '' })
  })

  expect(urlSearch()).toBe('')
})

it('treats undefined as clearing the filter', () => {
  renderProbe('/admin/users?search=ada')

  act(() => {
    hook.setFilters({ search: undefined })
  })

  expect(urlSearch()).toBe('')
})

it('resets page when another filter changes', () => {
  renderProbe('/admin/users?page=7')

  act(() => {
    hook.setFilters({ search: 'ada' })
  })

  // Page 7 of a narrowed result set is usually empty; the reset is the point.
  expect(urlSearch()).toBe('?search=ada')
  expect(hook.filters.page).toBe('1')
})

it('does not reset page when page itself is set', () => {
  renderProbe('/admin/users?search=ada')

  act(() => {
    hook.setFilters({ page: '2' })
  })

  expect(hook.filters).toMatchObject({ page: '2', search: 'ada' })
})

it('resets page even when page and another filter change together', () => {
  renderProbe('/admin/users?page=7')

  act(() => {
    hook.setFilters({ page: '9', search: 'ada' })
  })

  // A combined update still narrows the result set, so the explicit page loses.
  expect(hook.filters.page).toBe('1')
})

it('applies several filters at once', () => {
  renderProbe()

  act(() => {
    hook.setFilters({ search: 'ada', status: 'suspended' })
  })

  expect(param('search')).toBe('ada')
  expect(param('status')).toBe('suspended')
})

it('resetFilters clears every owned param', () => {
  renderProbe('/admin/users?search=ada&status=suspended&page=4')

  act(() => {
    hook.resetFilters()
  })

  expect(urlSearch()).toBe('')
  expect(hook.filters).toEqual(DEFAULTS)
})

it('leaves params it does not own untouched', () => {
  renderProbe('/admin/users?search=ada&tab=members')

  act(() => {
    hook.setFilters({ search: 'grace' })
  })
  expect(param('tab')).toBe('members')

  act(() => {
    hook.resetFilters()
  })
  expect(urlSearch()).toBe('?tab=members')
})

it('returns a referentially stable filters object across re-renders', () => {
  const { rerender } = renderProbe()

  const first = hook.filters
  rerender(<Probe />)

  // Consumers put `filters` in a fetch effect's deps; a fresh identity on every
  // render would refetch forever.
  expect(renderCount).toBeGreaterThan(1)
  expect(hook.filters).toBe(first)
  expect(filtersIdentityChanges).toBe(1)
})

function DefaultsProbe({ defaults }: Readonly<{ defaults: Filters }>) {
  // Mirrors how every caller invokes the hook: a fresh object literal per render.
  hook = useUrlFilters<Filters>({ ...defaults })
  const { search } = useLocation()
  return <span data-testid="search">{search}</span>
}

it('captures the defaults from the first render and ignores later ones', () => {
  const { rerender } = render(<DefaultsProbe defaults={DEFAULTS} />, {
    wrapper: ({ children }: Readonly<{ children: ReactNode }>) => (
      <MemoryRouter initialEntries={['/admin/users']}>{children}</MemoryRouter>
    ),
  })

  expect(hook.filters.status).toBe('all')

  rerender(<DefaultsProbe defaults={{ ...DEFAULTS, status: 'active' }} />)

  expect(hook.filters.status).toBe('all')

  // Not just on the memoized path: after a recompute the resolved defaults are
  // still the first render's, so `filters` cannot silently change meaning when
  // an unrelated param moves.
  act(() => {
    hook.setFilters({ search: 'ada' })
  })
  expect(hook.filters.status).toBe('all')

  // And the capture is the same one the "a value equal to its default stays out
  // of the URL" elision consults, so writing a filter agrees with reading it.
  // Against the later defaults, 'active' would have been elided as a default
  // and 'all' written as an override — exactly backwards.
  act(() => {
    hook.setFilters({ status: 'active' })
  })
  expect(param('status')).toBe('active')

  act(() => {
    hook.setFilters({ status: 'all' })
  })
  expect(param('status')).toBeNull()
  expect(param('search')).toBe('ada')
})

it('replaces rather than pushes history, so typing does not fill the stack', () => {
  renderProbe()

  for (const search of ['a', 'ad', 'ada']) {
    act(() => {
      hook.setFilters({ search })
    })
  }
  expect(urlSearch()).toBe('?search=ada')

  act(() => {
    goBack()
  })

  // Three replaces leave a single history entry, so there is nowhere to go back
  // to. Had they pushed, this would now read '?search=ad'.
  expect(urlSearch()).toBe('?search=ada')
})
