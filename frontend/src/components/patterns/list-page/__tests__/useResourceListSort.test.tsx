import { act, render } from '@testing-library/react'

import type { ResourceKindKey } from '@/components/patterns/resource'
import { getResourceDescriptor } from '@/components/patterns/resource'

import type { SortDir } from '../types'
import { useResourceListSort } from '../useResourceListSort'

const setFilters = vi.fn()

let captured: ReturnType<typeof useResourceListSort>

function Probe({
  kind,
  sortBy,
  sortOrder,
}: Readonly<{ kind: ResourceKindKey; sortBy: string; sortOrder: SortDir }>) {
  captured = useResourceListSort({
    descriptor: getResourceDescriptor(kind),
    sortBy,
    sortOrder,
    setFilters,
    fallback: 'updated_at',
  })
  return null
}

function renderSort(
  kind: ResourceKindKey,
  sortBy: string,
  sortOrder: SortDir = 'desc'
) {
  return render(<Probe kind={kind} sortBy={sortBy} sortOrder={sortOrder} />)
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useResourceListSort', () => {
  it.each([
    ['artifact', ['title', 'updated_at']],
    ['blueprint', ['title', 'updated_at']],
    ['memory', ['text', 'updated_at']],
    ['prompt', ['name', 'status', 'updated_at']],
  ] as [ResourceKindKey, string[]][])(
    'takes %s sortable keys from the descriptor',
    (kind, expected) => {
      renderSort(kind, 'updated_at')
      expect([...captured.sortableKeys]).toEqual(expected)
    }
  )

  it('accepts a declared key from the URL', () => {
    renderSort('prompt', 'status')
    expect(captured.sortKey).toBe('status')
  })

  it('falls back when the URL names a key the descriptor does not declare', () => {
    // `status` is sortable on prompts but not on artifacts, so this is the
    // exact drift the fallback exists for — the endpoint would answer 400.
    renderSort('artifact', 'status')
    expect(captured.sortKey).toBe('updated_at')
  })

  it('flips the direction when the active column is clicked again', () => {
    renderSort('blueprint', 'title', 'asc')
    act(() => {
      captured.onSortChange('title')
    })
    expect(setFilters).toHaveBeenCalledWith({
      sort_by: 'title',
      sort_order: 'desc',
    })
  })

  it('rewrites an undeclared sort_by out of the URL on the next click', () => {
    // Arrived as `?sort_by=status` on artifacts, which the endpoint 400s on.
    renderSort('artifact', 'status')
    act(() => {
      // The user clicks the header the page is actually showing as active.
      captured.onSortChange('updated_at')
    })
    expect(setFilters).toHaveBeenCalledWith({
      sort_by: 'updated_at',
      sort_order: 'asc',
    })
  })

  it('starts a new name column ascending', () => {
    renderSort('memory', 'updated_at')
    act(() => {
      captured.onSortChange('text')
    })
    expect(setFilters).toHaveBeenCalledWith({
      sort_by: 'text',
      sort_order: 'asc',
    })
  })

  it('starts any other new column descending', () => {
    renderSort('prompt', 'name', 'asc')
    act(() => {
      captured.onSortChange('updated_at')
    })
    expect(setFilters).toHaveBeenCalledWith({
      sort_by: 'updated_at',
      sort_order: 'desc',
    })
  })

  it('offers nothing sortable for a kind with no list section', () => {
    renderSort('gallery-prompt', 'updated_at')
    expect(captured.sortableKeys).toEqual([])
  })
})
