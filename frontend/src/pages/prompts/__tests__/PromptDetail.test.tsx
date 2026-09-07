import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Prompt } from '@/services/promptService'
import { storage } from '@/utils/storage'

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

// Radix primitives can loop/crash in JSDOM — replace with plain divs.
vi.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode
    value: string
  }) => <div data-value={value}>{children}</div>,
}))

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom (AccessActivityPanel in the sidebar renders a chart).
vi.mock('recharts', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 110 }}>{children}</div>
    ),
  }
})

vi.mock('@/services/promptService', () => ({
  promptService: {
    getPrompt: vi.fn(),
    getPromptDependencies: vi.fn(),
    getPromptVersions: vi.fn(),
    deletePrompt: vi.fn(),
  },
}))

// The sidebar self-fetching panels (attachments / access activity / comments)
// hit their services on mount — resolve them all to empty.
vi.mock('@/services/attachmentService', () => ({
  attachmentService: {
    list: vi.fn().mockResolvedValue({
      attachments: [],
      total_count: 0,
      total_size_bytes: 0,
    }),
    upload: vi.fn(),
    remove: vi.fn(),
    download: vi.fn(),
  },
}))
vi.mock('@/services/resourceAccessService', () => ({
  resourceAccessService: {
    getResourceAccessMetrics: vi.fn().mockResolvedValue({
      status: 'success',
      message: 'ok',
      data: { total_accesses: 0, range: '30d', counts: [] },
    }),
  },
}))
vi.mock('@/services/commentService', () => ({
  commentService: {
    list: vi.fn().mockResolvedValue({ comments: [], total_count: 0 }),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
  },
}))
vi.mock('@/services/teamService', () => ({
  teamService: {
    getTeamMembers: vi.fn().mockResolvedValue([]),
  },
}))

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

// Mutable so each test chooses the server-granted permissions array — the page
// gates delete on it via the real usePermissions hook (never mocked, #225).
const mockTeamState: {
  currentTeam: { id: string; name: string; permissions: string[] } | null
  isLoading: boolean
} = {
  currentTeam: { id: 'team-1', name: 'Test Team', permissions: [] },
  isLoading: false,
}
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: mockTeamState.isLoading,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn() as () => Promise<void>,
  }),
}))

// Stable object so PromptDetail's render effects do not loop; hoisted and
// mutable so a test can put the page into a placeholder / render-error state
// (`vi.hoisted` because the mock factory below is itself hoisted above it).
const mockRenderer = vi.hoisted(() => {
  const placeholderValues: Record<string, string> = {}
  return {
    renderedBody: '',
    renderError: null as string | null,
    isRendering: false,
    allPlaceholders: [] as string[],
    placeholderValues,
    isLoadingPlaceholders: false,
    renderPrompt: vi.fn(),
    fetchPlaceholders: vi.fn(),
    updatePlaceholderValue: vi.fn(),
  }
})

function resetRenderer() {
  mockRenderer.renderedBody = ''
  mockRenderer.renderError = null
  mockRenderer.isRendering = false
  mockRenderer.allPlaceholders = []
  mockRenderer.placeholderValues = {}
  mockRenderer.isLoadingPlaceholders = false
  mockRenderer.renderPrompt.mockResolvedValue(undefined)
  mockRenderer.fetchPlaceholders.mockResolvedValue(undefined)
}

vi.mock('@/hooks', () => {
  const showSuccess = vi.fn()
  const showError = vi.fn()
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
    usePromptRenderer: () => mockRenderer,
  }
})

const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import React from 'react'

import { promptService } from '@/services/promptService'

import { PromptDetail } from '../PromptDetail'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    id: 'prompt-1',
    name: 'Code Review Template',
    slug: 'code-review-template',
    description: 'Template for conducting code reviews',
    body: 'Please review this code for: {{criteria}}',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'proj-1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['code-review', 'documentation'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = { id: 'team-1', name: 'Test Team', permissions }
}

function renderPromptDetail(slug = 'code-review-template') {
  return render(
    <MemoryRouter initialEntries={[`/prompts/${slug}`]}>
      <Routes>
        <Route path="/prompts/:slug" element={<PromptDetail />} />
        <Route
          path="/prompts"
          element={<div data-testid="list-probe">Prompts list probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('PromptDetail page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetRenderer()
    storage.clear()
    setTeamPermissions([])
    ;(promptService.getPrompt as Mock).mockResolvedValue(buildPrompt())
    ;(promptService.getPromptDependencies as Mock).mockResolvedValue({
      used_by: [],
      uses: [],
    })
    ;(promptService.getPromptVersions as Mock).mockResolvedValue({
      versions: [],
    })
  })

  describe('happy render', () => {
    it('renders the fetched prompt title, status, slug and body', async () => {
      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      expect(promptService.getPrompt).toHaveBeenCalledWith(
        'team-1',
        'code-review-template'
      )
      // Status and slug each render twice since #903: once in the reading
      // header, once as a descriptor-generated Metadata row.
      expect(screen.getAllByText('published')).toHaveLength(2)
      expect(
        screen.getAllByText('code-review-template').length
      ).toBeGreaterThan(0)
      // The body renders once, in the default (Rendered) view — only the
      // active tab panel is mounted (#901).
      expect(screen.getByRole('tabpanel')).toHaveTextContent(
        'Please review this code for: {{criteria}}'
      )
    })

    it('shows a loading header while the fetch is in flight', () => {
      ;(promptService.getPrompt as Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderPromptDetail()

      expect(screen.getByText('Loading prompt…')).toBeInTheDocument()
    })
  })

  describe('nullable labels (#121 drift class)', () => {
    it('renders without crashing when labels is null and hides the Labels card', async () => {
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ labels: null })
      )

      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      expect(screen.queryByText('Labels')).not.toBeInTheDocument()
    })

    it('renders the Labels card when labels are present', async () => {
      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Labels')).toBeInTheDocument()
      })
      expect(screen.getByText('code-review')).toBeInTheDocument()
      expect(screen.getByText('documentation')).toBeInTheDocument()
    })
  })

  describe('delete gating — own vs any (#225)', () => {
    it('shows delete for the owner holding resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-1' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })

    it('hides delete for a non-owner holding only resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-2' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
      // Edit is not gated: every role holds resource.update.any.
      expect(screen.getByTestId('edit-prompt-button')).toBeInTheDocument()
    })

    it('shows delete for a non-owner holding resource.delete.any', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-2' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })
  })

  describe('versions and dependencies sections', () => {
    it('renders the version-history link with the snapshot count', async () => {
      ;(promptService.getPromptVersions as Mock).mockResolvedValue({
        versions: [
          { id: 'v2', version_number: 2 },
          { id: 'v1', version_number: 1 },
        ],
      })

      renderPromptDetail()

      const link = await screen.findByTestId('metadata-version-history-link')
      expect(link).toHaveTextContent('2')
      expect(link).toHaveAttribute(
        'href',
        '/prompts/code-review-template/versions'
      )
    })

    it('hides the version-history link when there are no snapshots', async () => {
      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
    })

    it('renders the "Used by" dependencies section from the service', async () => {
      ;(promptService.getPromptDependencies as Mock).mockResolvedValue({
        used_by: [
          { id: 'dep-1', slug: 'parent-prompt', name: 'Parent Prompt' },
        ],
        uses: [],
      })

      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Used by')).toBeInTheDocument()
      })
      expect(screen.getByText('Parent Prompt')).toBeInTheDocument()
    })

    it('still renders the prompt when versions and dependencies fetches fail', async () => {
      ;(promptService.getPromptVersions as Mock).mockRejectedValue(
        new Error('versions boom')
      )
      ;(promptService.getPromptDependencies as Mock).mockRejectedValue(
        new Error('deps boom')
      )

      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Used by')).not.toBeInTheDocument()
    })
  })

  describe('interactions', () => {
    it('deletes the prompt after confirmation and navigates back to the list', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(promptService.deletePrompt as Mock).mockResolvedValue(undefined)

      renderPromptDetail()

      const user = userEvent.setup()
      await user.click(await screen.findByTestId('delete-prompt-button'))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete prompt?')).toBeInTheDocument()
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(promptService.deletePrompt).toHaveBeenCalledWith(
          'team-1',
          'code-review-template'
        )
      })
      await waitFor(() => {
        expect(screen.getByTestId('list-probe')).toBeInTheDocument()
      })
    })

    it('copies the raw body when nothing has been rendered yet', async () => {
      renderPromptDetail()
      await screen.findByText('Code Review Template')

      // userEvent.setup installs a clipboard stub; read it back to observe.
      const user = userEvent.setup()
      // The Metadata section's slug chip is also a "Copy …" button (#903), so
      // address the page action by its exact name.
      await user.click(screen.getByRole('button', { name: 'Copy content' }))

      expect(await navigator.clipboard.readText()).toBe(
        'Please review this code for: {{criteria}}'
      )
    })
  })

  describe('service error', () => {
    it('reports the error and navigates back to the prompts list', async () => {
      ;(promptService.getPrompt as Mock).mockRejectedValue(
        new Error('not found')
      )

      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByTestId('list-probe')).toBeInTheDocument()
      })
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load prompt'
      )
    })
  })

  describe('body view switch (#901)', () => {
    it('shows the raw source in the Raw view and remembers the choice', async () => {
      const user = userEvent.setup()
      const first = renderPromptDetail()
      await screen.findByText('Code Review Template')

      const body = screen.getByTestId('resource-body')
      expect(within(body).getByTestId('markdown-renderer')).toBeInTheDocument()

      await user.click(within(body).getByRole('tab', { name: 'Raw' }))

      // `rawContent` is the prompt's own body — never the placeholder-rendered
      // output — and only that one view is mounted.
      expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(
        'Please review this code for: {{criteria}}'
      )
      expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()

      // The page owns the mode (its render effects key off it) but persists it
      // under the same shared key, so a remount comes back in Raw.
      first.unmount()
      renderPromptDetail()
      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('resource-body-raw')).toBeInTheDocument()
    })

    it('renders the placeholder inputs and render error through the rendered-only slot', async () => {
      const user = userEvent.setup()
      mockRenderer.allPlaceholders = ['criteria']
      mockRenderer.placeholderValues = { criteria: '' }
      mockRenderer.renderError = 'unknown placeholder'

      renderPromptDetail()
      await screen.findByText('Code Review Template')

      // Both live in `renderedExtra`; deleting that prop drops them entirely.
      expect(screen.getByPlaceholderText('Enter criteria')).toBeInTheDocument()
      expect(screen.getByText('Render error')).toBeInTheDocument()
      expect(screen.getByText('unknown placeholder')).toBeInTheDocument()

      // …and they belong to the Rendered view only.
      await user.click(screen.getByRole('tab', { name: 'Raw' }))
      expect(
        screen.queryByPlaceholderText('Enter criteria')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Render error')).not.toBeInTheDocument()
    })

    it('forwards placeholder edits to the renderer', async () => {
      const user = userEvent.setup()
      mockRenderer.allPlaceholders = ['criteria']
      mockRenderer.placeholderValues = { criteria: '' }

      renderPromptDetail()
      await screen.findByText('Code Review Template')

      await user.type(screen.getByPlaceholderText('Enter criteria'), 'a')

      expect(mockRenderer.updatePlaceholderValue).toHaveBeenCalledWith(
        'criteria',
        'a'
      )
    })
  })
})
