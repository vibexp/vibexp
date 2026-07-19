import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Prompt } from '@/services/promptService'

// The shared lucide mock lacks FileCode (PromptContentCard's raw-tab icon);
// extend it locally instead of editing the shared mock file.
jest.mock('lucide-react', () => {
  const actual = jest.requireActual<Record<string, unknown>>('lucide-react')
  const ReactActual = jest.requireActual<typeof import('react')>('react')
  const icon = (name: string) => (props: object) =>
    ReactActual.createElement('svg', {
      'data-testid': `${name.toLowerCase()}-icon`,
      ...props,
    })
  return {
    ...actual,
    FileCode: actual.FileCode ?? icon('FileCode'),
  }
})

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

// Radix primitives can loop/crash in JSDOM — replace with plain divs.
jest.mock('@/components/ui/tabs', () => ({
  Tabs: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="tabs">{children}</div>
  ),
  TabsList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  TabsTrigger: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  TabsContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))
jest.mock('@/components/ui/select', () => ({
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
jest.mock('recharts', () => {
  const actual = jest.requireActual<Record<string, unknown>>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 110 }}>{children}</div>
    ),
  }
})

jest.mock('@/services/promptService', () => ({
  promptService: {
    getPrompt: jest.fn(),
    getPromptDependencies: jest.fn(),
    getPromptVersions: jest.fn(),
    deletePrompt: jest.fn(),
  },
}))

// The sidebar self-fetching panels (attachments / access activity / comments)
// hit their services on mount — resolve them all to empty.
jest.mock('@/services/attachmentService', () => ({
  attachmentService: {
    list: jest
      .fn()
      .mockResolvedValue({
        attachments: [],
        total_count: 0,
        total_size_bytes: 0,
      }),
    upload: jest.fn(),
    remove: jest.fn(),
    download: jest.fn(),
  },
}))
jest.mock('@/services/resourceAccessService', () => ({
  resourceAccessService: {
    getResourceAccessMetrics: jest.fn().mockResolvedValue({
      status: 'success',
      message: 'ok',
      data: { total_accesses: 0, range: '30d', counts: [] },
    }),
  },
}))
jest.mock('@/services/commentService', () => ({
  commentService: {
    list: jest.fn().mockResolvedValue({ comments: [], total_count: 0 }),
    create: jest.fn(),
    update: jest.fn(),
    remove: jest.fn(),
  },
}))
jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamMembers: jest.fn().mockResolvedValue([]),
  },
}))

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
jest.mock('@/contexts/useAuth', () => ({
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
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: mockTeamState.isLoading,
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn() as () => Promise<void>,
  }),
}))

jest.mock('@/hooks', () => {
  const showSuccess = jest.fn()
  const showError = jest.fn()
  const trackEvent = jest.fn()
  // Stable object so PromptDetail's render effects do not loop.
  const renderer = {
    renderedBody: '',
    renderError: null,
    isRendering: false,
    allPlaceholders: [] as string[],
    placeholderValues: {} as Record<string, string>,
    isLoadingPlaceholders: false,
    renderPrompt: jest.fn().mockResolvedValue(undefined),
    fetchPlaceholders: jest.fn().mockResolvedValue(undefined),
    updatePlaceholderValue: jest.fn(),
  }
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
    usePromptRenderer: () => renderer,
  }
})

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
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
    jest.clearAllMocks()
    setTeamPermissions([])
    ;(promptService.getPrompt as jest.Mock).mockResolvedValue(buildPrompt())
    ;(promptService.getPromptDependencies as jest.Mock).mockResolvedValue({
      used_by: [],
      uses: [],
    })
    ;(promptService.getPromptVersions as jest.Mock).mockResolvedValue({
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
      expect(screen.getByText('published')).toBeInTheDocument()
      expect(screen.getByText('code-review-template')).toBeInTheDocument()
      // Raw body reaches the content card (both tab panels render because the
      // Tabs primitive is mocked with plain divs).
      expect(
        screen.getAllByText('Please review this code for: {{criteria}}').length
      ).toBeGreaterThan(0)
    })

    it('shows a loading header while the fetch is in flight', () => {
      ;(promptService.getPrompt as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderPromptDetail()

      expect(screen.getByText('Loading prompt…')).toBeInTheDocument()
    })
  })

  describe('nullable labels (#121 drift class)', () => {
    it('renders without crashing when labels is null and hides the Labels card', async () => {
      ;(promptService.getPrompt as jest.Mock).mockResolvedValue(
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
      ;(promptService.getPrompt as jest.Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-1' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })

    it('hides delete for a non-owner holding only resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(promptService.getPrompt as jest.Mock).mockResolvedValue(
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
      ;(promptService.getPrompt as jest.Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-2' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })
  })

  describe('versions and dependencies sections', () => {
    it('renders the version-history link with the snapshot count', async () => {
      ;(promptService.getPromptVersions as jest.Mock).mockResolvedValue({
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
      ;(promptService.getPromptDependencies as jest.Mock).mockResolvedValue({
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
      ;(promptService.getPromptVersions as jest.Mock).mockRejectedValue(
        new Error('versions boom')
      )
      ;(promptService.getPromptDependencies as jest.Mock).mockRejectedValue(
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
      ;(promptService.deletePrompt as jest.Mock).mockResolvedValue(undefined)

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
      await user.click(screen.getByRole('button', { name: /Copy/ }))

      expect(await navigator.clipboard.readText()).toBe(
        'Please review this code for: {{criteria}}'
      )
    })
  })

  describe('service error', () => {
    it('reports the error and navigates back to the prompts list', async () => {
      ;(promptService.getPrompt as jest.Mock).mockRejectedValue(
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
})
