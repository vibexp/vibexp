import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { PromptGalleryTemplate } from '@/services/promptGalleryService'

// The shared lucide mock does not export every icon this page uses (Wand2),
// so mock the whole module with a Proxy that fabricates any icon on demand.
jest.mock('lucide-react', () => {
  const ReactActual = jest.requireActual<typeof import('react')>('react')
  const iconCache = new Map<string, unknown>()
  return new Proxy(
    {},
    {
      get: (_target, prop: string | symbol) => {
        if (prop === '__esModule') return true
        const name = String(prop)
        if (!iconCache.has(name)) {
          const MockIcon = (props: Record<string, unknown>) =>
            ReactActual.createElement('svg', {
              'data-testid': `${name.toLowerCase()}-icon`,
              ...props,
            })
          MockIcon.displayName = name
          iconCache.set(name, MockIcon)
        }
        return iconCache.get(name)
      },
    }
  )
})

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues.
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-content">{content}</div>
  ),
}))

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

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={['/prompt-gallery/prompt/gallery-1']}>
      <Routes>
        <Route
          path="/prompt-gallery"
          element={<div data-testid="gallery-probe">Gallery probe</div>}
        />
        <Route
          path="/prompt-gallery/prompt/:id"
          element={<PromptGalleryDetail />}
        />
        <Route
          path="/prompt-gallery/:category"
          element={<div data-testid="category-probe">Category probe</div>}
        />
        <Route
          path="/prompts/new"
          element={<div data-testid="editor-probe">Prompt editor probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

const getPromptByIdMock = promptGalleryService.getPromptById as jest.Mock
const trackPromptUsageMock = promptGalleryService.trackPromptUsage as jest.Mock

describe('PromptGalleryDetail page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    getPromptByIdMock.mockResolvedValue(buildTemplate())
    trackPromptUsageMock.mockResolvedValue(undefined)
  })

  afterEach(() => {
    jest.useRealTimers()
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
    jest.useFakeTimers()
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
      jest.advanceTimersByTime(2000)
    })
    expect(screen.getByTestId('gallery-probe')).toBeInTheDocument()
  })
})
