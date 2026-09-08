import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Prompt } from '@/services/promptService'

const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router', async () => ({
  ...(await vi.importActual<typeof import('react-router')>('react-router')),
  useNavigate: () => mockNavigate,
}))

// Mock Radix Select — it can loop in JSDOM (same approach as Artifacts.test.tsx),
// but keep onValueChange wired so tests can still pick options as plain buttons.
vi.mock('@/components/ui/select', async () => {
  const ReactActual = await vi.importActual<typeof import('react')>('react')
  const SelectCtx = ReactActual.createContext<(value: string) => void>(() => {})
  return {
    Select: ({
      children,
      onValueChange,
    }: {
      children: React.ReactNode
      value: string
      onValueChange: (v: string) => void
    }) => (
      <SelectCtx.Provider value={onValueChange}>
        <div data-testid="select">{children}</div>
      </SelectCtx.Provider>
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
    }) => {
      const onValueChange = ReactActual.useContext(SelectCtx)
      return (
        <button
          type="button"
          data-value={value}
          onClick={() => {
            onValueChange(value)
          }}
        >
          {children}
        </button>
      )
    },
  }
})

// Functional Tabs mock: plain buttons that still forward onValueChange so the
// page's view switching stays testable without Radix in jsdom. All TabsContent
// panes render unconditionally, which is fine for these assertions.
vi.mock('@/components/ui/tabs', async () => {
  const ReactActual = await vi.importActual<typeof import('react')>('react')
  const TabsCtx = ReactActual.createContext<(value: string) => void>(() => {})
  return {
    Tabs: ({
      children,
      onValueChange,
    }: {
      children: React.ReactNode
      value: string
      onValueChange: (v: string) => void
    }) => (
      <TabsCtx.Provider value={onValueChange}>
        <div data-testid="tabs">{children}</div>
      </TabsCtx.Provider>
    ),
    TabsList: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    TabsTrigger: ({
      children,
      value,
    }: {
      children: React.ReactNode
      value: string
    }) => {
      const onValueChange = ReactActual.useContext(TabsCtx)
      return (
        <button
          type="button"
          data-testid={`tab-trigger-${value}`}
          onClick={() => {
            onValueChange(value)
          }}
        >
          {children}
        </button>
      )
    },
    TabsContent: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
  }
})

// The mention textarea pulls in prompt search + a Radix popover; a plain
// textarea keeps the body editable without any of that. It renders `error`
// itself — the shared editor hands the message over rather than rendering a
// second one beside it (#914), so a mock that dropped it would hide the
// body's validation message from every assertion here.
vi.mock('@/components/PromptMentionTextarea', () => ({
  PromptMentionTextarea: ({
    value,
    onChange,
    error,
  }: {
    value: string
    onChange: (v: string) => void
    error?: string
  }) => (
    <>
      <textarea
        data-testid="prompt-body-textarea"
        value={value}
        onChange={e => {
          onChange(e.target.value)
        }}
      />
      {error && <p>{error}</p>}
    </>
  ),
}))

vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-preview">{content}</div>
  ),
}))

// Template loader dialog → a bare button that hands back a fixed template.
vi.mock('@/components/PromptTemplateLoader', () => ({
  PromptTemplateLoader: ({
    isOpen,
    onSelectPrompt,
  }: {
    isOpen: boolean
    onSelectPrompt: (p: Prompt) => void
  }) =>
    isOpen ? (
      <button
        type="button"
        data-testid="template-loader-pick"
        onClick={() => {
          onSelectPrompt(mockTemplatePrompt)
        }}
      >
        Pick template
      </button>
    ) : null,
}))

vi.mock('@/services/promptService', () => ({
  promptService: {
    getPrompt: vi.fn(),
    getPrompts: vi.fn(),
    createPrompt: vi.fn(),
    updatePrompt: vi.fn(),
    getPromptPlaceholders: vi.fn(),
    renderPrompt: vi.fn(),
  },
}))

vi.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: vi.fn(),
  },
}))

vi.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

vi.mock('@/hooks', () => {
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
    useAnalytics: () => ({ trackEvent }),
  }
})

// The picker fetches and searches projects of its own. Stubbed to one button so
// the tests can choose a project without going through that machinery — the
// generated form's own suite covers the real wiring.
vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    value,
    onChange,
    'data-testid': testId,
  }: {
    value: string
    onChange: (id: string) => void
    'data-testid'?: string
  }) => (
    <button
      type="button"
      data-testid={testId}
      onClick={() => {
        onChange('p1')
      }}
    >
      {value || 'pick project'}
    </button>
  ),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
  },
}))

import React from 'react'

import { useAnalytics } from '@/hooks'
import { toast } from '@/lib/toast'
import { projectService } from '@/services/projectService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { PromptEditor } from '../PromptEditor'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    id: 'prompt-1',
    name: 'My Prompt',
    slug: 'my-prompt',
    description: 'A description',
    body: 'Hello {{name}}',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'p1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['review'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 2,
    ...overrides,
  }
}

const mockTemplatePrompt = vi.hoisted(() =>
  buildPrompt({
    name: 'Great Template',
    slug: 'great-template',
    description: 'Template description',
    body: 'Template body',
  })
)

const projectAlpha = {
  id: 'p1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

const projectBeta = { ...projectAlpha, id: 'p2', name: 'Beta Project' }

function renderEditor(
  initialEntry:
    string | { pathname: string; state?: unknown } = '/prompts/create'
) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/prompts/create" element={<PromptEditor />} />
        <Route path="/prompts/:slug/edit" element={<PromptEditor />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  ;(projectService.getProjects as Mock).mockResolvedValue({
    projects: [projectAlpha],
  })
  ;(promptService.getPrompts as Mock).mockResolvedValue({ prompts: [] })
  ;(promptService.createPrompt as Mock).mockResolvedValue(buildPrompt())
  ;(promptService.updatePrompt as Mock).mockResolvedValue(buildPrompt())
})

/** Everything a create needs before the schema will let it submit. */
async function fillRequiredFields(
  user: ReturnType<typeof userEvent.setup>,
  name = 'My Prompt'
) {
  await user.type(screen.getByTestId('prompt-name-input'), name)
  fireEvent.change(screen.getByTestId('prompt-body-textarea'), {
    target: { value: 'Hello {{name}}' },
  })
}

describe('PromptEditor — create mode', () => {
  it('renders the generated form and navigates back on Back', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(
      screen.getByRole('heading', { name: 'Create prompt' })
    ).toBeInTheDocument()
    expect(screen.getByTestId('resource-form')).toBeInTheDocument()
    expect(screen.getByTestId('prompt-name-input')).toHaveValue('')
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue('')
    expect(promptService.getPrompt).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /back/i }))
    expect(mockNavigate).toHaveBeenCalledWith('/prompts')
  })

  it('preselects the only project a team has', async () => {
    renderEditor()
    await waitFor(() => {
      expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
        'p1'
      )
    })
  })

  it('leaves the project empty when the team has more than one', async () => {
    ;(projectService.getProjects as Mock).mockResolvedValue({
      projects: [projectAlpha, projectBeta],
    })
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })
    expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
      'pick project'
    )
  })

  it('auto-generates the slug from the name and creates the prompt', async () => {
    const user = userEvent.setup()
    renderEditor()
    await waitFor(() => {
      expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
        'p1'
      )
    })

    await fillRequiredFields(user, 'My New Prompt')
    await waitFor(() => {
      expect(screen.getByTestId('prompt-slug-input')).toHaveValue(
        'my-new-prompt'
      )
    })

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.createPrompt).toHaveBeenCalledWith('team-1', {
        name: 'My New Prompt',
        slug: 'my-new-prompt',
        description: '',
        body: 'Hello {{name}}',
        project_id: 'p1',
        status: 'draft',
        mcp_expose: false,
        labels: [],
      })
    })
    expect(mockNavigate).toHaveBeenCalledWith('/prompts/my-new-prompt')
  })

  it('surfaces validation errors and does not save an empty form', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(screen.getByTestId('prompt-save-button'))

    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Body is required')).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('rejects a manually entered slug with invalid characters', async () => {
    const user = userEvent.setup()
    renderEditor()

    await fillRequiredFields(user)
    await user.clear(screen.getByTestId('prompt-slug-input'))
    await user.type(screen.getByTestId('prompt-slug-input'), 'Bad Slug!')
    await user.click(screen.getByTestId('prompt-save-button'))

    expect(
      await screen.findByText('Lowercase letters, numbers, and dashes only')
    ).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('rejects a description longer than 200 characters', async () => {
    const user = userEvent.setup()
    renderEditor()

    await fillRequiredFields(user)
    fireEvent.change(screen.getByTestId('prompt-description-input'), {
      target: { value: 'x'.repeat(201) },
    })
    await user.click(screen.getByTestId('prompt-save-button'))

    expect(
      await screen.findByText('Description must be at most 200 characters')
    ).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('carries the status, the labels and the MCP toggle into the payload', async () => {
    const user = userEvent.setup()
    renderEditor()
    await waitFor(() => {
      expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
        'p1'
      )
    })

    await fillRequiredFields(user)
    await user.type(screen.getByTestId('prompt-labels-input'), 'review')
    await user.keyboard('{Enter}')

    // The MCP switch only exists once the prompt is published — the slot reads
    // the live status out of the form rather than mirroring it in the page.
    expect(screen.queryByTestId('prompt-mcp-expose')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Published' }))
    await user.click(await screen.findByTestId('prompt-mcp-expose'))

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.createPrompt).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({
          status: 'published',
          mcp_expose: true,
          labels: ['review'],
        })
      )
    })
  })

  it('never exposes a draft over MCP, however the toggle was left', async () => {
    const user = userEvent.setup()
    renderEditor()
    await waitFor(() => {
      expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
        'p1'
      )
    })

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: 'Published' }))
    await user.click(await screen.findByTestId('prompt-mcp-expose'))
    // …and back to draft, which withdraws the switch without resetting it.
    await user.click(screen.getByRole('button', { name: 'Draft' }))
    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.createPrompt).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ status: 'draft', mcp_expose: false })
      )
    })
  })

  it('prefills the form from navigation state', () => {
    renderEditor({
      pathname: '/prompts/create',
      state: {
        title: 'Gallery Prompt',
        body: 'Gallery body',
        description: 'Gallery description',
      },
    })

    expect(screen.getByTestId('prompt-name-input')).toHaveValue(
      'Based on: Gallery Prompt'
    )
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
      'Gallery body'
    )
  })

  it('loads a template into the form via the template loader', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(screen.getByRole('button', { name: /load template/i }))
    await user.click(screen.getByTestId('template-loader-pick'))

    await waitFor(() => {
      expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
        'Template body'
      )
    })
    expect(screen.getByTestId('prompt-name-input')).toHaveValue(
      'Great Template (Copy)'
    )
  })

  it('asks for confirmation before a template overwrites existing content', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    renderEditor()

    await user.type(screen.getByTestId('prompt-body-textarea'), 'typed')
    await waitFor(() => {
      expect(screen.getByTestId('prompt-body-textarea')).toHaveValue('typed')
    })
    await user.click(screen.getByRole('button', { name: /load template/i }))
    await user.click(screen.getByTestId('template-loader-pick'))

    expect(confirmSpy).toHaveBeenCalled()
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue('typed')
    confirmSpy.mockRestore()
  })

  it('keeps the fields a template does not set when one is loaded', async () => {
    const user = userEvent.setup()
    renderEditor()
    await waitFor(() => {
      expect(screen.getByTestId('prompt-project-select')).toHaveTextContent(
        'p1'
      )
    })

    await user.type(screen.getByTestId('prompt-labels-input'), 'review')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: /load template/i }))
    await user.click(screen.getByTestId('template-loader-pick'))

    // Re-seeding the form is a `reset`, so the template has to carry the rest
    // of the form with it or the project and the labels vanish.
    await waitFor(() => {
      expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
        'Template body'
      )
    })
    expect(screen.getByText('review')).toBeInTheDocument()
    expect(screen.getByTestId('prompt-project-select')).toHaveTextContent('p1')
  })
})

describe('PromptEditor — edit mode', () => {
  it('prefills the form from the loaded prompt and updates on save', async () => {
    const user = userEvent.setup()
    ;(promptService.getPrompt as Mock).mockResolvedValue(buildPrompt())
    renderEditor('/prompts/my-prompt/edit')

    await waitFor(() => {
      expect(screen.getByTestId('prompt-name-input')).toHaveValue('My Prompt')
    })
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
      'Hello {{name}}'
    )
    expect(screen.getByText('review')).toBeInTheDocument()
    expect(screen.getByText('Edit prompt')).toBeInTheDocument()
    // The project is never re-fetched for an edit: the prompt carries its own.
    expect(projectService.getProjects).not.toHaveBeenCalled()

    await user.clear(screen.getByTestId('prompt-name-input'))
    await user.type(screen.getByTestId('prompt-name-input'), 'Renamed')
    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.updatePrompt).toHaveBeenCalledWith(
        'team-1',
        'my-prompt',
        expect.objectContaining({
          name: 'Renamed',
          // The slug does NOT follow a rename after create: a prompt is
          // addressed by slug alone.
          slug: 'my-prompt',
          mcp_expose: true,
        })
      )
    })
    expect(mockNavigate).toHaveBeenCalledWith('/prompts/my-prompt')
  })

  it('opens the render view, loads placeholders, and shows the rendered output', async () => {
    const user = userEvent.setup()
    ;(promptService.getPrompt as Mock).mockResolvedValue(buildPrompt())
    ;(promptService.getPromptPlaceholders as Mock).mockResolvedValue(['name'])
    ;(promptService.renderPrompt as Mock).mockResolvedValue({
      rendered_body: 'Hello world',
    })
    renderEditor('/prompts/my-prompt/edit')

    await waitFor(() => {
      expect(screen.getByTestId('prompt-name-input')).toHaveValue('My Prompt')
    })

    await user.click(screen.getByTestId('tab-trigger-render'))

    await waitFor(() => {
      expect(promptService.getPromptPlaceholders).toHaveBeenCalledWith(
        'team-1',
        'my-prompt'
      )
    })
    expect(
      await screen.findByPlaceholderText('Enter value for {{name}}')
    ).toBeInTheDocument()
  })

  it('shows an error toast and navigates back when the prompt fails to load', async () => {
    ;(promptService.getPrompt as Mock).mockRejectedValue(new Error('nope'))
    renderEditor('/prompts/my-prompt/edit')

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled()
    })
    expect(mockNavigate).toHaveBeenCalledWith('/prompts')
  })

  it('surfaces a save failure without navigating away', async () => {
    const user = userEvent.setup()
    ;(promptService.getPrompt as Mock).mockResolvedValue(buildPrompt())
    ;(promptService.updatePrompt as Mock).mockRejectedValue(
      new Error('save failed')
    )
    renderEditor('/prompts/my-prompt/edit')

    await waitFor(() => {
      expect(screen.getByTestId('prompt-name-input')).toHaveValue('My Prompt')
    })

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled()
    })
    expect(mockNavigate).not.toHaveBeenCalledWith('/prompts/my-prompt')
  })
})

describe('PromptEditor — analytics', () => {
  it('tracks the preview view', async () => {
    const user = userEvent.setup()
    const { trackEvent } = useAnalytics()
    renderEditor()

    await user.click(screen.getByTestId('tab-trigger-preview'))

    expect(trackEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        event: ANALYTICS_EVENTS.PROMPT_PREVIEW_VIEWED,
      })
    )
  })
})
