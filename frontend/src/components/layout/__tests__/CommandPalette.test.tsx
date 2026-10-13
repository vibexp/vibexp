import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mocked } from 'vitest'

import type { SearchResultItem } from '@/services/searchService'

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1', name: 'Team' } }),
}))
vi.mock('@/services/searchService', () => ({
  searchService: { search: vi.fn() },
}))

import { searchService } from '@/services/searchService'

import { CommandPalette, PALETTE_DEBOUNCE_MS } from '../CommandPalette'
import { groupResults, searchPagePath } from '../commandPaletteResults'

const mockSearch = (searchService as Mocked<typeof searchService>).search

function item(overrides: Partial<SearchResultItem>): SearchResultItem {
  return {
    type: 'artifact',
    id: 'a1',
    title: 'Release checklist',
    slug: 'release-checklist',
    project_id: 'p1',
    project_name: 'Project 1',
    ...overrides,
  } as SearchResultItem
}

function response(results: SearchResultItem[]) {
  return {
    results,
    total_count: results.length,
    page: 1,
    per_page: 12,
    total_pages: 1,
  }
}

function Where() {
  const location = useLocation()
  return (
    <div data-testid="where">{`${location.pathname}${location.search}`}</div>
  )
}

function renderPalette() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Where />
      <CommandPalette />
    </MemoryRouter>
  )
}

/** Types and waits out the debounce, as a reader pausing would. */
async function typeQuery(text: string) {
  const user = userEvent.setup()
  await user.type(screen.getByRole('combobox'), text)
  await act(async () => {
    await new Promise(resolve => setTimeout(resolve, PALETTE_DEBOUNCE_MS + 50))
  })
  return user
}

beforeAll(() => {
  // cmdk scrolls the active item into view; jsdom has no layout.
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockSearch.mockResolvedValue(response([]))
})

describe('CommandPalette', () => {
  it('opens from the header button, on a single-line input', async () => {
    const user = userEvent.setup()
    renderPalette()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Search' }))

    const input = screen.getByRole('combobox')
    expect(input.tagName).toBe('INPUT')
    expect(input).toHaveFocus()
  })

  it.each([['{Control>}k{/Control}'], ['{Meta>}k{/Meta}']])(
    'opens and closes on %s',
    async keys => {
      const user = userEvent.setup()
      renderPalette()

      await user.keyboard(keys)
      expect(screen.getByRole('dialog')).toBeInTheDocument()

      await user.keyboard(keys)
      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      })
    }
  )

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))

    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('offers quick navigation to the main pages before anything is typed', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))

    const options = screen.getAllByRole('option').map(o => o.textContent)
    expect(options).toEqual(
      expect.arrayContaining(['Dashboard', 'Artifacts', 'Memories', 'Connect'])
    )
    expect(mockSearch).not.toHaveBeenCalled()

    await user.click(screen.getByRole('option', { name: 'Artifacts' }))
    expect(screen.getByTestId('where')).toHaveTextContent('/artifacts')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('searches as you type, once per pause, and groups results by type with their project', async () => {
    mockSearch.mockResolvedValue(
      response([
        item({ type: 'memory', id: 'm1', title: 'Vitest mock hoisting trap' }),
        item({ type: 'artifact', id: 'a1', title: 'Release checklist' }),
        item({ type: 'memory', id: 'm2', title: 'Hoisting in ESM' }),
      ])
    )
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))

    await typeQuery('hoisting')

    // Debounced: one request for the settled query, not one per keystroke.
    expect(mockSearch).toHaveBeenCalledTimes(1)
    expect(mockSearch).toHaveBeenCalledWith(
      'team-1',
      expect.objectContaining({ query: 'hoisting' })
    )
    const artifacts = await screen.findByRole('group', { name: 'Artifacts' })
    const memories = screen.getByRole('group', { name: 'Memories' })
    expect(within(artifacts).getByRole('option')).toHaveTextContent(
      'Release checklistProject 1'
    )
    expect(
      within(memories)
        .getAllByRole('option')
        .map(o => o.textContent)
    ).toEqual([
      'Vitest mock hoisting trapProject 1',
      'Hoisting in ESMProject 1',
    ])
  })

  it('moves with the arrow keys and opens the highlighted result on Enter', async () => {
    mockSearch.mockResolvedValue(
      response([
        item({ type: 'artifact', id: 'a1', slug: 'one', title: 'One' }),
        item({ type: 'artifact', id: 'a2', slug: 'two', title: 'Two' }),
      ])
    )
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await typeQuery('doc')
    await screen.findByRole('option', { name: /Two/ })

    // The first result is active; one step down is the second.
    await user.keyboard('{ArrowDown}{Enter}')

    expect(screen.getByTestId('where')).toHaveTextContent('/artifacts/p1/two')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('ends with a row that takes the query to the full results page', async () => {
    mockSearch.mockResolvedValue(response([item({})]))
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await typeQuery('path params')
    await screen.findByRole('option', { name: /Release checklist/ })

    const options = screen.getAllByRole('option')
    expect(options.at(-1)).toHaveTextContent(
      'See all results for “path params”'
    )

    await user.click(options.at(-1)!)
    expect(screen.getByTestId('where')).toHaveTextContent(
      '/search?q=path%20params'
    )
  })

  it('still offers the full results page when nothing matches', async () => {
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await typeQuery('zzzz')

    expect(await screen.findByText('No matches.')).toBeInTheDocument()
    expect(screen.getByTestId('command-palette-see-all')).toBeInTheDocument()
  })

  it('says so when the search fails, without breaking the palette', async () => {
    mockSearch.mockRejectedValue(new Error('boom'))
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await typeQuery('anything')

    expect(
      await screen.findByText('Search is unavailable right now.')
    ).toBeInTheDocument()
    expect(screen.getByTestId('command-palette-see-all')).toBeInTheDocument()
  })

  it('offers no AI summary: that stays on the full search page', async () => {
    mockSearch.mockResolvedValue(response([item({})]))
    const user = userEvent.setup()
    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await typeQuery('release')
    await screen.findByRole('option', { name: /Release checklist/ })

    expect(screen.queryByText(/AI Summary/i)).not.toBeInTheDocument()
  })
})

describe('groupResults', () => {
  it('lists each resource once, by its best-ranked hit', () => {
    const groups = groupResults([
      item({ id: 'a1', title: 'First chunk' }),
      item({ id: 'a1', title: 'Second chunk' }),
      item({ id: 'a2', title: 'Other' }),
    ])

    expect(groups).toHaveLength(1)
    expect(groups[0].items.map(i => i.title)).toEqual(['First chunk', 'Other'])
  })

  it('orders groups prompts, artifacts, blueprints, memories and drops empty ones', () => {
    const groups = groupResults([
      item({ type: 'memory', id: 'm1' }),
      item({ type: 'prompt', id: 'p1' }),
      item({ type: 'artifact', id: 'a1' }),
    ])

    expect(groups.map(g => g.heading)).toEqual([
      'Prompts',
      'Artifacts',
      'Memories',
    ])
  })
})

describe('searchPagePath', () => {
  it('encodes the query', () => {
    expect(searchPagePath('a & b')).toBe('/search?q=a%20%26%20b')
  })
})
