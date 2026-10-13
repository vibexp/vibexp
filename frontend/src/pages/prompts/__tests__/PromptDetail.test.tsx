import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import { openMoreActions } from '@/components/patterns/reading-page/testing'
import type { Prompt } from '@/services/promptService'
import { storage } from '@/utils/storage'

// The team's status catalog (#1263), settable per test. Empty and answered by
// default — a team with no custom statuses.
const statusCatalog = vi.hoisted(() => ({
  statuses: [] as unknown[],
  isLoading: false,
}))
vi.mock('@/hooks/useStatusCatalog', () => ({
  useStatusCatalog: () => statusCatalog,
}))
afterEach(() => {
  statusCatalog.statuses = []
})

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({
    content,
    placeholders,
  }: {
    content: string
    placeholders?: Record<string, string>
  }) => (
    <div
      data-testid="markdown-renderer"
      data-placeholders={JSON.stringify(placeholders)}
    >
      {content}
    </div>
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
    lockPrompt: vi.fn(),
    unlockPrompt: vi.fn(),
    getPromptLockHistory: vi.fn().mockResolvedValue([]),
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

// Mocked only to prove it is never called: the Project row reads the payload's
// `project` summary (#956), where a list lookup was capped at 100 projects.
vi.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: vi.fn(),
  },
}))

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

import { projectService } from '@/services/projectService'
import { promptService } from '@/services/promptService'

import { PromptDetail } from '../PromptDetail'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    lock: null,
    id: 'prompt-1',
    name: 'Code Review Template',
    slug: 'code-review-template',
    description: 'Template for conducting code reviews',
    body: 'Please review this code for: {{criteria}}',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'proj-1',
    project: { id: 'proj-1', name: 'My Project', slug: 'my-project-slug' },
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
    it('shows a custom status by its display name, once, in the header (#1263)', async () => {
      statusCatalog.statuses = [
        { slug: 'in-review', name: 'In review', is_system: false },
      ]
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ status: 'in-review' })
      )
      renderPromptDetail()

      const header = await screen.findByTestId('resource-header-meta')
      expect(within(header).getByText('In review')).toHaveClass('bg-muted')
      // Once: the rail's details no longer repeat what the header shows.
      expect(screen.getAllByText('In review')).toHaveLength(1)
      expect(screen.queryByText('in-review')).not.toBeInTheDocument()
    })

    it('shows the raw slug for a status the catalog no longer has (#1263)', async () => {
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ status: 'in-review' })
      )
      renderPromptDetail()

      const header = await screen.findByTestId('resource-header-meta')
      expect(within(header).getByText('in-review')).toHaveClass('bg-muted')
      expect(screen.getAllByText('in-review')).toHaveLength(1)
    })

    it('renders the fetched prompt title, status, slug and body', async () => {
      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      expect(promptService.getPrompt).toHaveBeenCalledWith(
        'team-1',
        'code-review-template'
      )
      // The status is shown once, in the reading header, and the slug once, in
      // the rail's details — both through the descriptor's `valueLabels`, not
      // the raw enum (#902).
      expect(screen.getAllByText('Published')).toHaveLength(1)
      expect(screen.queryByText('published')).not.toBeInTheDocument()
      expect(
        screen.getAllByText('code-review-template').length
      ).toBeGreaterThan(0)
      // The body renders once, in the default (Rendered) view — only the
      // active tab panel is mounted (#901).
      expect(screen.getByRole('tabpanel')).toHaveTextContent(
        'Please review this code for: {{criteria}}'
      )
    })

    it('links the Metadata Project row from the payload summary without listing projects (#956)', async () => {
      renderPromptDetail()

      const link = await screen.findByRole('link', { name: /My Project/ })
      expect(link).toHaveAttribute(
        'href',
        '/teams/team-1/projects/my-project-slug/edit'
      )
      expect(projectService.getProjects).not.toHaveBeenCalled()
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
    it('renders without crashing when labels is null and shows no taxonomy section', async () => {
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ labels: null })
      )

      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      // A prompt carries no metadata bag, so with no labels the whole unified
      // section is absent — the single emptiness rule (#904).
      expect(screen.queryByText('Labels')).not.toBeInTheDocument()
      expect(screen.queryByText('Labels & metadata')).not.toBeInTheDocument()
    })

    it('renders the labels as taxonomy chips when they are present', async () => {
      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Labels')).toBeInTheDocument()
      })
      expect(screen.getByText('Labels & metadata')).toBeInTheDocument()
      expect(screen.getByText('code-review')).toBeInTheDocument()
      expect(screen.getByText('documentation')).toBeInTheDocument()
    })
  })

  describe('unified taxonomy section (#904)', () => {
    it('drops the bespoke Description panel and shows the summary in the header', async () => {
      renderPromptDetail()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      // "Description" was a PanelTitle of its own before #904; the text now
      // lives in the standard reading header (#902) and nowhere else.
      expect(screen.queryByText('Description')).not.toBeInTheDocument()
      expect(screen.getByTestId('resource-header-summary')).toHaveTextContent(
        'Template for conducting code reviews'
      )
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
      await openMoreActions()
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })

    it('hides delete for a non-owner holding only resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(promptService.getPrompt as Mock).mockResolvedValue(
        buildPrompt({ user_id: 'user-2' })
      )

      renderPromptDetail()

      await screen.findByText('Code Review Template')
      // Opened first: Delete missing from a closed menu would prove nothing.
      await openMoreActions()
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
      await openMoreActions()
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
      await screen.findByText('Code Review Template')
      await openMoreActions()
      await user.click(screen.getByTestId('delete-prompt-button'))

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
    it('shows the raw source in the Raw view, and opens on Rendered again next time', async () => {
      const user = userEvent.setup()
      const first = renderPromptDetail()
      await screen.findByText('Code Review Template')

      const body = screen.getByTestId('resource-body')
      expect(within(body).getByTestId('markdown-renderer')).toBeInTheDocument()

      await user.click(screen.getByRole('tab', { name: 'Raw' }))

      // `rawContent` is the prompt's own body — never the placeholder-rendered
      // output — and only that one view is mounted.
      expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(
        'Please review this code for: {{criteria}}'
      )
      expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()

      // Raw lasts for that page view only: the next visit opens on Rendered,
      // with the placeholder treatment the Rendered view carries.
      first.unmount()
      renderPromptDetail()
      await screen.findByText('Code Review Template')
      expect(screen.queryByTestId('resource-body-raw')).not.toBeInTheDocument()
      expect(screen.getByTestId('markdown-renderer')).toBeInTheDocument()
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

    it('asks the server to keep every placeholder and substitutes the values on screen', async () => {
      mockRenderer.allPlaceholders = ['criteria']
      mockRenderer.placeholderValues = { criteria: 'security' }
      mockRenderer.fetchPlaceholders.mockResolvedValue(undefined)

      renderPromptDetail()
      await screen.findByText('Code Review Template')

      // An empty value sent to the server would erase the placeholder from the
      // body, so none are sent at all…
      await waitFor(() => {
        expect(mockRenderer.renderPrompt).toHaveBeenCalledWith(
          'code-review-template',
          'team-1',
          {}
        )
      })
      // …and the values reach the markdown renderer instead.
      expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
        'data-placeholders',
        JSON.stringify({ criteria: 'security' })
      )
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

  describe('resource lock (#1291)', () => {
    const mockGet = (value: Prompt) => {
      ;(promptService.getPrompt as Mock).mockResolvedValue(value)
    }
    beforeEach(() => {
      ;(promptService.getPromptLockHistory as Mock).mockResolvedValue([])
    })

    const lockedBy = (userId: string) => ({
      locked_by: { user_id: userId, display_name: 'Ada Lovelace' },
      comment: 'Used by the nightly release agent',
      locked_at: '2026-10-11T10:15:30Z',
    })

    it('offers Lock and Delete on an unlocked prompt, with no lock row', async () => {
      setTeamPermissions(['resource.delete.own'])
      mockGet(buildPrompt())

      renderPromptDetail()

      await screen.findByTestId('resource-body')
      await openMoreActions()
      expect(screen.getByTestId('lock-prompt-button')).toBeInTheDocument()
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
      expect(
        screen.queryByTestId('unlock-prompt-button')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Locked by')).not.toBeInTheDocument()
    })

    it('shows who locked it and hides Lock, Unlock and Delete from another member', async () => {
      // The reader created the prompt and may delete their own, so only the
      // lock is hiding Delete here.
      setTeamPermissions(['resource.delete.own'])
      mockGet({ ...buildPrompt(), lock: lockedBy('user-2') })

      renderPromptDetail()

      expect(await screen.findByTestId('locked-by')).toHaveTextContent(
        'Ada Lovelace'
      )
      expect(screen.getByText('Locked by')).toBeInTheDocument()
      expect(screen.queryByTestId('lock-prompt-button')).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('unlock-prompt-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
    })

    it('offers Unlock to the locker, still without Delete', async () => {
      setTeamPermissions(['resource.delete.own'])
      mockGet({ ...buildPrompt(), lock: lockedBy('user-1') })

      renderPromptDetail()

      await screen.findByTestId('locked-by')
      await openMoreActions()
      expect(screen.getByTestId('unlock-prompt-button')).toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
    })

    it("offers Unlock on someone else's lock to a holder of resource.unlock.any, read from permissions", async () => {
      setTeamPermissions(['resource.unlock.any', 'resource.delete.any'])
      mockGet({ ...buildPrompt(), lock: lockedBy('user-2') })

      renderPromptDetail()

      await screen.findByTestId('locked-by')
      await openMoreActions()
      expect(screen.getByTestId('unlock-prompt-button')).toBeInTheDocument()
      // Not even resource.delete.any deletes a locked prompt.
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
    })

    it('locks from the dialog and shows the lock without refetching; Delete returns after unlock', async () => {
      const user = userEvent.setup()
      setTeamPermissions(['resource.delete.own'])
      mockGet(buildPrompt())
      ;(promptService.lockPrompt as Mock).mockResolvedValue({
        ...buildPrompt(),
        lock: lockedBy('user-1'),
      })
      ;(promptService.unlockPrompt as Mock).mockResolvedValue(buildPrompt())

      renderPromptDetail()

      await screen.findByTestId('resource-body')
      await openMoreActions()
      await user.click(screen.getByTestId('lock-prompt-button'))
      // The dialog does not submit without a comment.
      expect(screen.getByTestId('confirm-lock-button')).toBeDisabled()
      await user.type(
        screen.getByTestId('lock-comment-input'),
        'Used by the nightly release agent'
      )
      await user.click(screen.getByTestId('confirm-lock-button'))

      expect(await screen.findByTestId('locked-by')).toHaveTextContent(
        'Ada Lovelace'
      )
      expect(promptService.lockPrompt).toHaveBeenCalledWith(
        'team-1',
        'code-review-template',
        'Used by the nightly release agent'
      )
      await openMoreActions()
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
      expect(screen.queryByTestId('lock-prompt-button')).not.toBeInTheDocument()
      expect(promptService.getPrompt).toHaveBeenCalledTimes(1)

      await user.click(screen.getByTestId('unlock-prompt-button'))

      await waitFor(() => {
        expect(screen.queryByTestId('locked-by')).not.toBeInTheDocument()
      })
      await openMoreActions()
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
      expect(promptService.unlockPrompt).toHaveBeenCalledWith(
        'team-1',
        'code-review-template'
      )
      expect(screen.queryByText('Locked by')).not.toBeInTheDocument()
      expect(promptService.getPrompt).toHaveBeenCalledTimes(1)
    })

    it('lists the lock history in its own section', async () => {
      setTeamPermissions([])
      mockGet({ ...buildPrompt(), lock: lockedBy('user-2') })
      ;(promptService.getPromptLockHistory as Mock).mockResolvedValue([
        {
          id: 'e-2',
          action: 'lock',
          actor: { user_id: 'user-2', display_name: 'Ada Lovelace' },
          comment: 'Used by the nightly release agent',
          created_at: '2026-10-11T10:15:30Z',
        },
        {
          id: 'e-1',
          action: 'unlock',
          actor: null,
          comment: null,
          created_at: '2026-10-10T10:15:30Z',
        },
      ])

      renderPromptDetail()

      const events = await screen.findAllByTestId('lock-history-event')
      expect(promptService.getPromptLockHistory).toHaveBeenCalledWith(
        'team-1',
        'code-review-template'
      )
      expect(events.map(event => event.dataset.action)).toEqual([
        'lock',
        'unlock',
      ])
      expect(events[0]).toHaveTextContent(/Ada Lovelace\s*locked/)
      expect(events[1]).toHaveTextContent(/Deleted user\s*unlocked/)
      expect(
        within(screen.getByTestId('lock-history')).getByText('Lock history')
      ).toBeInTheDocument()
    })
  })
})
