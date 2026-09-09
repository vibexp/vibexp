import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router'
import type { Mock } from 'vitest'

import type { PromptGalleryTemplate } from '@/services/promptGalleryService'
import { storage } from '@/utils/storage'

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues.
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-content">{content}</div>
  ),
}))

vi.mock('@/services/promptGalleryService', () => ({
  promptGalleryService: {
    getCategories: vi.fn(),
    getPrompts: vi.fn(),
    getPromptById: vi.fn(),
    trackPromptUsage: vi.fn(),
  },
}))

const mockShowAlert = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/AlertContext', () => ({
  useAlertContext: () => ({ showAlert: mockShowAlert }),
}))

import { promptGalleryService } from '@/services/promptGalleryService'

import { PromptGalleryDetail } from '../PromptGalleryDetail'

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

/** Surfaces the live URL so a `replace` redirect can be asserted. */
function LocationProbe() {
  const location = useLocation()
  const navigate = useNavigate()
  return (
    <>
      <div data-testid="location">{location.pathname}</div>
      <button
        type="button"
        onClick={() => {
          void navigate(-1)
        }}
      >
        history back
      </button>
    </>
  )
}

const CANONICAL_ENTRY = '/prompt-gallery/Engineering/gallery-1'

function renderDetail(initialEntries: string[] = [CANONICAL_ENTRY]) {
  return render(
    <MemoryRouter
      initialEntries={initialEntries}
      initialIndex={initialEntries.length - 1}
    >
      <LocationProbe />
      <Routes>
        <Route
          path="/prompt-gallery"
          element={<div data-testid="gallery-probe">Gallery probe</div>}
        />
        <Route
          path="/prompt-gallery/:category"
          element={<div data-testid="category-probe">Category probe</div>}
        />
        <Route
          path="/prompt-gallery/:category/:id"
          element={<PromptGalleryDetail />}
        />
        <Route
          path="/prompts/new"
          element={<div data-testid="editor-probe">Prompt editor probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

const getPromptByIdMock = promptGalleryService.getPromptById as Mock
const trackPromptUsageMock = promptGalleryService.trackPromptUsage as Mock

describe('PromptGalleryDetail page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getPromptByIdMock.mockResolvedValue(buildTemplate())
    trackPromptUsageMock.mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows a loading header while the prompt is in flight', () => {
    getPromptByIdMock.mockImplementation(() => new Promise(() => undefined))

    renderDetail()

    expect(screen.getByText('Loading prompt…')).toBeInTheDocument()
  })

  it('renders the prompt content, category and tags', async () => {
    renderDetail()

    await waitFor(() => {
      expect(screen.getByText('Code Review Request')).toBeInTheDocument()
    })
    expect(getPromptByIdMock).toHaveBeenCalledWith('gallery-1')
    expect(screen.getByTestId('markdown-content')).toHaveTextContent(
      'Please review the following code'
    )
    expect(screen.getByText('Engineering')).toBeInTheDocument()
    expect(screen.getByText('security')).toBeInTheDocument()
    expect(screen.getByText('quality')).toBeInTheDocument()
    expect(screen.getByTestId('copy-button')).toBeInTheDocument()
  })

  it('renders Category and Tags through the descriptor taxonomy section (#917)', async () => {
    renderDetail()

    // Both are `taxonomy` fields on the `gallery-prompt` descriptor, so they
    // arrive as one generated block — the page hand-builds no Panel.
    const taxonomy = await screen.findByTestId('taxonomy-section')
    expect(within(taxonomy).getByText('Category')).toBeInTheDocument()
    expect(within(taxonomy).getByText('Engineering')).toBeInTheDocument()
    expect(within(taxonomy).getByText('Tags')).toBeInTheDocument()
    expect(within(taxonomy).getByText('security')).toBeInTheDocument()
    expect(within(taxonomy).getByText('quality')).toBeInTheDocument()
  })

  it('renders the standard header: description and updated time (#902)', async () => {
    renderDetail()

    const header = await screen.findByTestId('resource-header-meta')
    // A gallery template has no team-scoped status or slug, so the shared
    // header renders only its updated time and summary.
    expect(within(header).getByText('Updated')).toBeInTheDocument()
    expect(
      within(header).getByText('Request a thorough code review')
    ).toBeInTheDocument()
    expect(within(header).queryByRole('button')).not.toBeInTheDocument()
  })

  it('tracks usage and pre-fills the prompt editor from Use this prompt', async () => {
    renderDetail()
    await screen.findByText('Code Review Request')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Use this prompt/ }))

    await waitFor(() => {
      expect(trackPromptUsageMock).toHaveBeenCalledWith('gallery-1')
    })
    expect(screen.getByTestId('editor-probe')).toBeInTheDocument()
  })

  it('stays on the page and alerts when usage tracking fails', async () => {
    trackPromptUsageMock.mockRejectedValue(new Error('usage failed'))

    renderDetail()
    await screen.findByText('Code Review Request')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Use this prompt/ }))

    await waitFor(() => {
      expect(mockShowAlert).toHaveBeenCalledWith({
        type: 'error',
        message: 'usage failed',
      })
    })
    expect(screen.queryByTestId('editor-probe')).not.toBeInTheDocument()
  })

  it('navigates back to the prompt category from the Back button', async () => {
    renderDetail()
    await screen.findByText('Code Review Request')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Back/ }))

    expect(screen.getByTestId('category-probe')).toBeInTheDocument()
  })

  it('alerts, shows not-found and redirects to the gallery when the fetch fails', async () => {
    vi.useFakeTimers()
    getPromptByIdMock.mockRejectedValue(new Error('prompt gone'))

    renderDetail()

    // Flush the rejected fetch without relying on real timers.
    await act(async () => {
      await Promise.resolve()
    })

    expect(mockShowAlert).toHaveBeenCalledWith({
      type: 'error',
      message: 'prompt gone',
    })
    // Rendered both as the page title and the alert title.
    expect(screen.getAllByText('Prompt not found').length).toBeGreaterThan(0)

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(screen.getByTestId('gallery-probe')).toBeInTheDocument()
  })

  describe('the retired /prompt-gallery/prompt/:id path (#920)', () => {
    it('rewrites itself onto the category-nested URL once the prompt loads', async () => {
      renderDetail(['/prompt-gallery/prompt/gallery-1'])

      await waitFor(() => {
        expect(screen.getByTestId('location').textContent).toBe(CANONICAL_ENTRY)
      })
      // Same page, same fetch - only the URL was normalised.
      expect(getPromptByIdMock).toHaveBeenCalledWith('gallery-1')
      expect(screen.getByText('Code Review Request')).toBeInTheDocument()
    })

    it('replaces the legacy entry so Back skips past it', async () => {
      renderDetail([
        '/prompt-gallery/Engineering',
        '/prompt-gallery/prompt/gallery-1',
      ])

      await waitFor(() => {
        expect(screen.getByTestId('location').textContent).toBe(CANONICAL_ENTRY)
      })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'history back' }))

      // Not `/prompt-gallery/prompt/gallery-1`: `replace` dropped it, so one
      // Back leaves the detail page instead of bouncing through the redirect.
      await waitFor(() => {
        expect(screen.getByTestId('location').textContent).toBe(
          '/prompt-gallery/Engineering'
        )
      })
      expect(screen.getByTestId('category-probe')).toBeInTheDocument()
    })

    it('encodes a category that needs it', async () => {
      getPromptByIdMock.mockResolvedValue(
        buildTemplate({ category: 'Code Review' })
      )

      renderDetail(['/prompt-gallery/prompt/gallery-1'])

      await waitFor(() => {
        expect(screen.getByTestId('location').textContent).toBe(
          '/prompt-gallery/Code%20Review/gallery-1'
        )
      })
    })

    it('leaves an already-canonical URL alone', async () => {
      renderDetail()

      await screen.findByText('Code Review Request')
      expect(screen.getByTestId('location').textContent).toBe(CANONICAL_ENTRY)
    })
  })

  describe('body view switch (#901)', () => {
    beforeEach(() => {
      storage.clear()
    })

    it('renders the gallery prompt body through ResourceBody, with a Raw view of the source', async () => {
      const user = userEvent.setup()
      renderDetail()

      // Rendered by default, through the shared body — not a bare renderer.
      const body = await screen.findByTestId('resource-body')
      expect(within(body).getByTestId('markdown-content')).toHaveTextContent(
        'Please review the following code'
      )

      await user.click(within(body).getByRole('tab', { name: 'Raw' }))

      expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(
        'Please review the following code'
      )
      // Exactly one view is mounted, so the body is never in the DOM twice.
      expect(screen.queryByTestId('markdown-content')).not.toBeInTheDocument()
    })
  })
})
