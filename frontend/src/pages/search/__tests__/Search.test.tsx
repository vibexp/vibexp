import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mock } from 'vitest'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import type {
  SearchResultItem,
  SearchResultsResponse,
} from '@/services/searchService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// A stable handleError identity across renders mirrors the real hook
// (its returned callback is memoized with useCallback). Returning a fresh
// vi.fn() per render would change the identity every render and re-run the
// fetch effect indefinitely.
const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

const mockCan = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/usePermissions', () => ({
  usePermissions: () => ({ can: mockCan }),
}))

vi.mock('@/services/searchService', () => ({
  searchService: { search: vi.fn(), summarize: vi.fn() },
}))

vi.mock('@/services/projectService', () => ({
  projectService: { getProjects: vi.fn() },
}))

import { projectService } from '@/services/projectService'
import { searchService } from '@/services/searchService'
import { storage } from '@/utils/storage'

import { summaryKey } from '../aiSummary'
import { Search } from '../Search'
import {
  requestSearchSummary,
  resetSearchSummaries,
} from '../searchSummaryStore'

const mockSearch = searchService.search as Mock
const mockSummarize = searchService.summarize as Mock
const mockGetProjects = projectService.getProjects as Mock

function makeItem(overrides: Partial<SearchResultItem>): SearchResultItem {
  return {
    type: 'prompt',
    id: 'id-1',
    title: 'Default title',
    excerpt: 'short excerpt',
    score: 0.5,
    chunk_id: 'chunk-1',
    updated_at: '2024-01-01T00:00:00Z',
    slug: '',
    project_id: '',
    project_name: '',
    ...overrides,
  }
}

function makeResponse(results: SearchResultItem[]): SearchResultsResponse {
  return {
    results,
    total_count: results.length,
    page: 1,
    per_page: 20,
    total_pages: 1,
  }
}

function renderSearch(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Search />
    </MemoryRouter>
  )
}

describe('Search page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetSearchSummaries()
    storage.remove(STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED)
    mockCan.mockReturnValue(false)
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    mockGetProjects.mockResolvedValue({
      projects: [
        { id: 'proj-1', name: 'Project One' },
        { id: 'proj-2', name: 'Project Two' },
      ],
      total_count: 2,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })
  })

  it('does not call the search service when the query is empty', async () => {
    renderSearch('/search')

    // findBy flushes the async projects-loading effect within act().
    expect(await screen.findByText('Type to search')).toBeInTheDocument()
    expect(mockSearch).not.toHaveBeenCalled()
  })

  it('does not call the search service when the query is whitespace', async () => {
    renderSearch('/search?q=%20%20')

    expect(await screen.findByText('Type to search')).toBeInTheDocument()
    expect(mockSearch).not.toHaveBeenCalled()
  })

  it('renders the type label as the title for memory results', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'memory',
          id: 'mem-1',
          title: 'unused',
          chunk_id: 'chunk-mem',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    expect(
      await screen.findByRole('heading', { name: 'Memory' })
    ).toBeInTheDocument()
    expect(mockSearch).toHaveBeenCalledWith('team-1', {
      query: 'foo',
      page: 1,
      per_page: 20,
    })
  })

  it('renders the item title for non-memory results', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'prompt',
          title: 'A Real Prompt',
          slug: 'a-real-prompt',
          chunk_id: 'chunk-prompt',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    expect(
      await screen.findByRole('heading', { name: 'A Real Prompt' })
    ).toBeInTheDocument()
  })

  it('links prompt results to /prompts/:slug', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({ type: 'prompt', slug: 'my-prompt', chunk_id: 'c1' }),
      ])
    )

    renderSearch('/search?q=foo')

    const link = await screen.findByRole('link')
    expect(link).toHaveAttribute('href', '/prompts/my-prompt')
  })

  it('links artifact results to /artifacts/:projectId/:slug', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'artifact',
          title: 'Art',
          slug: 'art-slug',
          project_id: 'proj-uuid',
          chunk_id: 'c2',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    const link = await screen.findByRole('link')
    expect(link).toHaveAttribute('href', '/artifacts/proj-uuid/art-slug')
  })

  it('links memory results to /memories/:id', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([makeItem({ type: 'memory', id: 'mem-42', chunk_id: 'c3' })])
    )

    renderSearch('/search?q=foo')

    const link = await screen.findByRole('link')
    expect(link).toHaveAttribute('href', '/memories/mem-42')
  })

  it('renders a non-clickable card when a non-memory result is missing its slug', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'prompt',
          title: 'No Slug Prompt',
          slug: '',
          chunk_id: 'c4',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    expect(
      await screen.findByRole('heading', { name: 'No Slug Prompt' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('truncates a long excerpt and expands it inline on click', async () => {
    const longExcerpt = 'x'.repeat(300)
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'prompt',
          title: 'Long',
          slug: 'long',
          excerpt: longExcerpt,
          chunk_id: 'c5',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    const showMore = await screen.findByRole('button', { name: 'Show more' })
    expect(screen.getByText(`${'x'.repeat(200)}…`)).toBeInTheDocument()
    expect(screen.queryByText(longExcerpt)).not.toBeInTheDocument()

    await userEvent.click(showMore)

    expect(screen.getByText(longExcerpt)).toBeInTheDocument()
    const showLess = screen.getByRole('button', { name: 'Show less' })
    expect(showLess).toBeInTheDocument()

    await userEvent.click(showLess)

    expect(screen.queryByText(longExcerpt)).not.toBeInTheDocument()
    expect(screen.getByText(`${'x'.repeat(200)}…`)).toBeInTheDocument()
  })

  it('fetches the next page when pagination advances', async () => {
    mockSearch.mockResolvedValue({
      results: [makeItem({ type: 'prompt', slug: 'p', chunk_id: 'c7' })],
      total_count: 40,
      page: 1,
      per_page: 20,
      total_pages: 2,
    })

    renderSearch('/search?q=foo')

    const next = await screen.findByRole('button', { name: 'Next' })
    await userEvent.click(next)

    await waitFor(() => {
      expect(mockSearch).toHaveBeenLastCalledWith('team-1', {
        query: 'foo',
        page: 2,
        per_page: 20,
      })
    })
  })

  it('shows an empty state when there are no matches', async () => {
    mockSearch.mockResolvedValue(makeResponse([]))

    renderSearch('/search?q=foo')

    expect(await screen.findByText('No matches found')).toBeInTheDocument()
  })

  it('shows an error state when the search fails', async () => {
    mockSearch.mockRejectedValue(new Error('network down'))

    renderSearch('/search?q=foo')

    expect(await screen.findByText('network down')).toBeInTheDocument()
  })

  it('renders nothing when there is no current team', () => {
    mockUseTeam.mockReturnValue({
      currentTeam: null,
      teams: [],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })

    const { container } = renderSearch('/search?q=foo')

    expect(mockSearch).not.toHaveBeenCalled()
    expect(container).toBeEmptyDOMElement()
  })

  it('does not show a stale link element while loading new results', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([makeItem({ type: 'prompt', slug: 'p', chunk_id: 'c6' })])
    )

    renderSearch('/search?q=foo')

    await waitFor(() => {
      expect(mockSearch).toHaveBeenCalled()
    })
  })

  it('shows the project name on each result', async () => {
    mockSearch.mockResolvedValue(
      makeResponse([
        makeItem({
          type: 'artifact',
          title: 'Art',
          slug: 'art-slug',
          project_id: 'proj-uuid',
          project_name: 'My Project',
          chunk_id: 'c8',
        }),
      ])
    )

    renderSearch('/search?q=foo')

    expect(await screen.findByText('My Project')).toBeInTheDocument()
  })

  it('forwards the type filter from the URL to the search request', async () => {
    mockSearch.mockResolvedValue(makeResponse([]))

    renderSearch('/search?q=foo&type=artifacts')

    await waitFor(() => {
      expect(mockSearch).toHaveBeenCalledWith('team-1', {
        query: 'foo',
        page: 1,
        per_page: 20,
        types: ['artifacts'],
      })
    })
  })

  it('forwards the project filter from the URL to the search request', async () => {
    mockSearch.mockResolvedValue(makeResponse([]))

    renderSearch('/search?q=foo&project=proj-1')

    await waitFor(() => {
      expect(mockSearch).toHaveBeenCalledWith('team-1', {
        query: 'foo',
        page: 1,
        per_page: 20,
        project_id: 'proj-1',
      })
    })
  })

  it('runs a new search when a query is submitted from the page search box', async () => {
    mockSearch.mockResolvedValue(makeResponse([]))

    renderSearch('/search?q=foo')

    await waitFor(() => {
      expect(mockSearch).toHaveBeenCalledWith('team-1', {
        query: 'foo',
        page: 1,
        per_page: 20,
      })
    })

    const input = screen.getByLabelText('Search query')
    await userEvent.clear(input)
    await userEvent.type(input, 'bar')
    await userEvent.click(screen.getByRole('button', { name: 'Search' }))

    await waitFor(() => {
      expect(mockSearch).toHaveBeenLastCalledWith('team-1', {
        query: 'bar',
        page: 1,
        per_page: 20,
      })
    })
  })

  describe('AI Summary', () => {
    const summary = {
      summary: 'The answer [1].',
      sources: [
        {
          index: 1,
          type: 'memory' as const,
          id: 'mem-1',
          title: 'Cited memory',
          slug: '',
          project_id: 'proj-1',
          project_name: 'Project One',
          updated_at: '2024-01-01T00:00:00Z',
          truncated: false,
        },
      ],
      model: 'gpt-test',
      provider_id: 'prov-1',
      generated_at: '2024-01-01T00:00:00Z',
    }

    function withSummary(
      aiSummary: SearchResultsResponse['ai_summary'],
      overrides: Partial<SearchResultsResponse> = {}
    ): SearchResultsResponse {
      return {
        ...makeResponse([
          makeItem({ type: 'memory', id: 'mem-1', chunk_id: 'c-mem' }),
        ]),
        ai_summary: aiSummary,
        ...overrides,
      }
    }

    const trigger = () => screen.findByRole('button', { name: /AI Summary/ })

    beforeEach(() => {
      mockSummarize.mockResolvedValue(summary)
    })

    it('makes no summary request while collapsed', async () => {
      mockSearch.mockResolvedValue(
        withSummary({ available: true, enabled: true })
      )
      renderSearch('/search?q=foo')

      expect(await trigger()).toHaveAttribute('aria-expanded', 'false')
      expect(mockSummarize).not.toHaveBeenCalled()
    })

    it('generates exactly once across expand, collapse and expand', async () => {
      mockSearch.mockResolvedValue(
        withSummary({ available: true, enabled: true })
      )
      renderSearch('/search?q=foo&type=memories')

      await userEvent.click(await trigger())
      expect(await screen.findByText('Generated by gpt-test')).toBeVisible()
      await userEvent.click(await trigger())
      await userEvent.click(await trigger())
      expect(await screen.findByText('Generated by gpt-test')).toBeVisible()

      expect(mockSummarize).toHaveBeenCalledTimes(1)
      expect(mockSummarize).toHaveBeenCalledWith('team-1', {
        query: 'foo',
        types: ['memories'],
      })
    })

    it('does not regenerate when paging', async () => {
      mockSearch.mockResolvedValue(
        withSummary(
          { available: true, enabled: true },
          { total_count: 40, total_pages: 2 }
        )
      )
      renderSearch('/search?q=foo')

      await userEvent.click(await trigger())
      expect(await screen.findByText('Generated by gpt-test')).toBeVisible()

      await userEvent.click(screen.getByRole('button', { name: 'Next' }))
      await waitFor(() => {
        expect(mockSearch).toHaveBeenLastCalledWith('team-1', {
          query: 'foo',
          page: 2,
          per_page: 20,
        })
      })
      expect(await screen.findByText('Generated by gpt-test')).toBeVisible()
      expect(mockSummarize).toHaveBeenCalledTimes(1)
    })

    it('generates a new summary for a new query', async () => {
      mockSearch.mockResolvedValue(
        withSummary({ available: true, enabled: true })
      )
      renderSearch('/search?q=foo')

      await userEvent.click(await trigger())
      expect(await screen.findByText('Generated by gpt-test')).toBeVisible()

      const input = screen.getByLabelText('Search query')
      await userEvent.clear(input)
      await userEvent.type(input, 'bar')
      await userEvent.click(screen.getByRole('button', { name: 'Search' }))

      await waitFor(() => {
        expect(mockSummarize).toHaveBeenCalledTimes(2)
      })
      expect(mockSummarize).toHaveBeenLastCalledWith('team-1', {
        query: 'bar',
      })
    })

    it('highlights a cited result on the page', async () => {
      // jsdom has no layout, so Element has no scrollIntoView to spy on.
      const scrollIntoView = vi.fn()
      Object.defineProperty(Element.prototype, 'scrollIntoView', {
        configurable: true,
        value: scrollIntoView,
      })
      onTestFinished(() => {
        Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
      })
      mockSearch.mockResolvedValue(
        withSummary({ available: true, enabled: true })
      )
      const { container } = renderSearch('/search?q=foo')

      await userEvent.click(await trigger())
      await userEvent.click(await screen.findByRole('link', { name: '[1]' }))

      const card = container.querySelector('#search-result-mem-1')
      expect(card).toHaveAttribute('data-highlighted', 'true')
      expect(scrollIntoView).toHaveBeenCalled()
    })

    it('hides the section from a member when no provider is configured', async () => {
      mockSearch.mockResolvedValue(
        withSummary({ available: false, enabled: true })
      )
      renderSearch('/search?q=foo')

      expect(
        await screen.findByRole('heading', { name: 'Memory' })
      ).toBeVisible()
      expect(
        screen.queryByRole('button', { name: /AI Summary/ })
      ).not.toBeInTheDocument()
      expect(screen.queryByText(/Configure a model provider/)).toBeNull()
      expect(mockCan).toHaveBeenCalledWith('team.update')
    })

    it('shows the configure hint to a team.update holder', async () => {
      mockCan.mockImplementation(
        (permission: string) => permission === 'team.update'
      )
      mockSearch.mockResolvedValue(
        withSummary({ available: false, enabled: true })
      )
      renderSearch('/search?q=foo')

      expect(
        await screen.findByRole('link', { name: /Configure a model provider/ })
      ).toHaveAttribute('href', '/teams/team-1/settings/model-providers')
    })

    describe('arriving from the header dialog (summary=open)', () => {
      function LocationProbe() {
        const location = useLocation()
        return <output data-testid="location">{location.search}</output>
      }

      async function primeCache(query: string) {
        requestSearchSummary(
          summaryKey('team-1', query, undefined, undefined),
          'team-1',
          { query }
        )
        await waitFor(() => {
          expect(mockSummarize).toHaveBeenCalledTimes(1)
        })
        mockSummarize.mockClear()
      }

      function renderArrival(entry: string) {
        return render(
          <MemoryRouter initialEntries={[entry]}>
            <Search />
            <LocationProbe />
          </MemoryRouter>
        )
      }

      it('opens the section with the cached answer and no new request', async () => {
        await primeCache('foo')
        mockSearch.mockResolvedValue(
          withSummary({ available: true, enabled: true })
        )
        renderArrival('/search?q=foo&summary=open')

        expect(await trigger()).toHaveAttribute('aria-expanded', 'true')
        expect(await screen.findByText('Generated by gpt-test')).toBeVisible()
        expect(mockSummarize).not.toHaveBeenCalled()
        // Consumed, so a refresh follows the stored preference again...
        expect(screen.getByTestId('location')).toHaveTextContent('?q=foo')
        expect(screen.getByTestId('location')).not.toHaveTextContent('summary')
        // ...which the override never wrote.
        expect(
          storage.getJSON(STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED)
        ).not.toBe(true)
      })

      it('lets the user collapse the pre-expanded section', async () => {
        await primeCache('foo')
        mockSearch.mockResolvedValue(
          withSummary({ available: true, enabled: true })
        )
        renderArrival('/search?q=foo&summary=open')

        await userEvent.click(await trigger())

        expect(await trigger()).toHaveAttribute('aria-expanded', 'false')
      })

      it('does not carry the override to the next query', async () => {
        mockSearch.mockResolvedValue(
          withSummary({ available: true, enabled: true })
        )
        renderArrival('/search?q=foo&summary=open')
        expect(await trigger()).toHaveAttribute('aria-expanded', 'true')

        const input = screen.getByLabelText('Search query')
        await userEvent.clear(input)
        await userEvent.type(input, 'bar')
        await userEvent.click(screen.getByRole('button', { name: 'Search' }))

        await waitFor(() => {
          expect(mockSearch).toHaveBeenLastCalledWith('team-1', {
            query: 'bar',
            page: 1,
            per_page: 20,
          })
        })
        expect(await trigger()).toHaveAttribute('aria-expanded', 'false')
        expect(mockSummarize).toHaveBeenCalledTimes(1)
      })
    })
  })
})
