import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { SearchResultItem, SearchResultsResponse } from '@/types/search'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// A stable handleError identity across renders mirrors the real hook
// (its returned callback is memoized with useCallback). Returning a fresh
// jest.fn() per render would change the identity every render and re-run the
// fetch effect indefinitely.
const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

jest.mock('@/services/searchService', () => ({
  searchService: { search: jest.fn() },
}))

jest.mock('@/services/projectService', () => ({
  projectService: { getProjects: jest.fn() },
}))

import { projectService } from '@/services/projectService'
import { searchService } from '@/services/searchService'

import { Search } from '../Search'

const mockSearch = searchService.search as jest.Mock
const mockGetProjects = projectService.getProjects as jest.Mock

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
    jest.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn() as () => Promise<void>,
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
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn() as () => Promise<void>,
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
})
