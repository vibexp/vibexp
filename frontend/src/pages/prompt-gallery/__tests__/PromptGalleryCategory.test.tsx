import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  PromptGalleryListResponse,
  PromptGalleryTemplate,
} from '@/services/promptGalleryService'

jest.mock('@/services/promptGalleryService', () => ({
  promptGalleryService: {
    getCategories: jest.fn(),
    getPrompts: jest.fn(),
    getPromptById: jest.fn(),
    trackPromptUsage: jest.fn(),
  },
}))

const mockShowAlert = jest.fn()
jest.mock('@/contexts/AlertContext', () => ({
  useAlertContext: () => ({ showAlert: mockShowAlert }),
}))

import { promptGalleryService } from '@/services/promptGalleryService'

import { PromptGalleryCategory } from '../PromptGalleryCategory'

function buildTemplate(
  overrides: Partial<PromptGalleryTemplate> = {}
): PromptGalleryTemplate {
  return {
    id: 'gallery-1',
    title: 'Code Review Request',
    description: 'Request a thorough code review',
    content: 'Please review the following code',
    category: 'Engineering',
    tags: ['security', 'quality'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  }
}

function buildListResponse(
  prompts: PromptGalleryTemplate[],
  overrides: Partial<PromptGalleryListResponse> = {}
): PromptGalleryListResponse {
  return {
    prompts,
    total_count: prompts.length,
    page: 1,
    per_page: 10,
    total_pages: prompts.length > 0 ? 1 : 0,
    ...overrides,
  }
}

function renderCategory(initialEntry = '/prompt-gallery/Engineering') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route
          path="/prompt-gallery"
          element={<div data-testid="gallery-probe">Gallery probe</div>}
        />
        <Route
          path="/prompt-gallery/prompt/:id"
          element={<div data-testid="detail-probe">Detail probe</div>}
        />
        <Route
          path="/prompt-gallery/:category"
          element={<PromptGalleryCategory />}
        />
      </Routes>
    </MemoryRouter>
  )
}

const getPromptsMock = promptGalleryService.getPrompts as jest.Mock

describe('PromptGalleryCategory page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    window.scrollTo = jest.fn()
    getPromptsMock.mockResolvedValue(buildListResponse([]))
  })

  it('fetches the decoded category and renders prompt cards with the count', async () => {
    getPromptsMock.mockResolvedValue(
      buildListResponse([
        buildTemplate(),
        buildTemplate({
          id: 'gallery-2',
          title: 'Bug Report Template',
          tags: ['triage'],
        }),
      ])
    )

    renderCategory('/prompt-gallery/Code%20Review')

    await waitFor(() => {
      expect(screen.getAllByTestId('gallery-prompt-card')).toHaveLength(2)
    })
    expect(getPromptsMock).toHaveBeenCalledWith({
      category: 'Code Review',
      search: undefined,
      tags: undefined,
      page: 1,
      limit: 10,
    })
    // Decoded category is the page title; count line comes from total_count.
    expect(screen.getByText('Code Review')).toBeInTheDocument()
    expect(screen.getByText('2 prompts available')).toBeInTheDocument()
    expect(screen.getByText('Code Review Request')).toBeInTheDocument()
    expect(screen.getByText('Bug Report Template')).toBeInTheDocument()
  })

  it('collects the distinct tags across prompts, sorted alphabetically', async () => {
    getPromptsMock.mockResolvedValue(
      buildListResponse([
        buildTemplate({ tags: ['zeta', 'alpha'] }),
        buildTemplate({ id: 'gallery-2', tags: ['alpha', 'mid'] }),
      ])
    )

    renderCategory()

    await waitFor(() => {
      expect(screen.getByText('Tags')).toBeInTheDocument()
    })
    const tagButtons = screen
      .getAllByRole('button')
      .filter(b => ['alpha', 'mid', 'zeta'].includes(b.textContent ?? ''))
    expect(tagButtons.map(b => b.textContent)).toEqual(['alpha', 'mid', 'zeta'])
  })

  it('re-fetches with the selected tag and clears it on a second toggle', async () => {
    getPromptsMock.mockResolvedValue(
      buildListResponse([buildTemplate({ tags: ['security'] })])
    )

    renderCategory()
    await screen.findByText('Tags')

    const user = userEvent.setup()
    const tagFilter = screen
      .getAllByRole('button')
      .find(b => b.textContent === 'security')
    expect(tagFilter).toBeDefined()
    await user.click(tagFilter!)

    await waitFor(() => {
      expect(getPromptsMock).toHaveBeenCalledWith(
        expect.objectContaining({ tags: ['security'], page: 1 })
      )
    })

    // Active tag renders with an inline clear icon; toggling removes the filter.
    const activeTag = screen
      .getAllByRole('button')
      .find(b => b.textContent?.startsWith('security'))
    await user.click(activeTag!)

    await waitFor(() => {
      const lastCall = getPromptsMock.mock.calls.at(-1)?.[0] as {
        tags?: string[]
      }
      expect(lastCall.tags).toBeUndefined()
    })
  })

  it('re-fetches with the search term and resets to page 1', async () => {
    getPromptsMock.mockResolvedValue(buildListResponse([buildTemplate()]))

    renderCategory()
    await screen.findByText('Code Review Request')

    const user = userEvent.setup()
    await user.type(
      screen.getByPlaceholderText('Search prompts by title or description…'),
      'api'
    )

    await waitFor(() => {
      expect(getPromptsMock).toHaveBeenCalledWith(
        expect.objectContaining({ search: 'api', page: 1 })
      )
    })
  })

  it('clears search and tag filters via the Clear filters button', async () => {
    getPromptsMock.mockResolvedValue(buildListResponse([buildTemplate()]))

    renderCategory()
    await screen.findByText('Code Review Request')

    const user = userEvent.setup()
    await user.type(
      screen.getByPlaceholderText('Search prompts by title or description…'),
      'x'
    )

    const clearButton = await screen.findByTestId('clear-filters-button')
    await user.click(clearButton)

    await waitFor(() => {
      const lastCall = getPromptsMock.mock.calls.at(-1)?.[0] as {
        search?: string
        tags?: string[]
      }
      expect(lastCall.search).toBeUndefined()
      expect(lastCall.tags).toBeUndefined()
    })
    expect(screen.queryByTestId('clear-filters-button')).not.toBeInTheDocument()
  })

  it('shows the filtered empty state with a clear action when filters are active', async () => {
    getPromptsMock.mockResolvedValueOnce(
      buildListResponse([buildTemplate({ tags: ['security'] })])
    )
    getPromptsMock.mockResolvedValue(buildListResponse([]))

    renderCategory()
    await screen.findByText('Tags')

    const user = userEvent.setup()
    const tagFilter = screen
      .getAllByRole('button')
      .find(b => b.textContent === 'security')
    await user.click(tagFilter!)

    await waitFor(() => {
      expect(screen.getByText('No prompts found')).toBeInTheDocument()
    })
    expect(
      screen.getByText('Try adjusting your filters or search terms.')
    ).toBeInTheDocument()
    const emptyState = screen.getByTestId('empty-state')
    expect(
      within(emptyState).getByTestId('clear-filters-button')
    ).toBeInTheDocument()
  })

  it('shows the unfiltered empty state without a clear action', async () => {
    renderCategory()

    await waitFor(() => {
      expect(screen.getByText('No prompts found')).toBeInTheDocument()
    })
    expect(
      screen.getByText('No prompts available in this category.')
    ).toBeInTheDocument()
    expect(screen.queryByTestId('clear-filters-button')).not.toBeInTheDocument()
  })

  it('pages forward and back through multi-page results', async () => {
    getPromptsMock.mockResolvedValue(
      buildListResponse([buildTemplate()], { total_count: 25, total_pages: 3 })
    )

    renderCategory()
    await screen.findByText('Page 1 of 3')

    const user = userEvent.setup()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(getPromptsMock).toHaveBeenCalledWith(
        expect.objectContaining({ page: 2 })
      )
    })
    await screen.findByText('Page 2 of 3')
    expect(window.scrollTo).toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Previous' }))
    await waitFor(() => {
      expect(screen.getByText('Page 1 of 3')).toBeInTheDocument()
    })
  })

  it('navigates to the prompt detail when a card is clicked', async () => {
    getPromptsMock.mockResolvedValue(buildListResponse([buildTemplate()]))

    renderCategory()

    const user = userEvent.setup()
    await user.click(await screen.findByTestId('gallery-prompt-card'))

    expect(screen.getByTestId('detail-probe')).toBeInTheDocument()
  })

  it('navigates back to the gallery from the Back button', async () => {
    renderCategory()
    await screen.findByText('No prompts found')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Back/ }))

    expect(screen.getByTestId('gallery-probe')).toBeInTheDocument()
  })

  it('surfaces a fetch failure through the alert context', async () => {
    getPromptsMock.mockRejectedValue(new Error('prompts unavailable'))

    renderCategory()

    await waitFor(() => {
      expect(mockShowAlert).toHaveBeenCalledWith({
        type: 'error',
        message: 'prompts unavailable',
      })
    })
  })
})
