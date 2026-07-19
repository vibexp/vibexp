import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Prompt } from '@/services/promptService'

// The shared lucide mock (tests/mocks/lucide-react.tsx) lists icons explicitly
// and misses Play/Wand2 used by EditorPane — serve any icon via a Proxy instead.
jest.mock(
  'lucide-react',
  () =>
    new Proxy(
      {},
      {
        get: (_target, name) => {
          if (name === '__esModule') return true
          const Icon = (props: object) => (
            <svg
              data-testid={`${String(name).toLowerCase()}-icon`}
              {...props}
            />
          )
          return Icon
        },
      }
    )
)

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

// Mock Radix Select — it can loop in JSDOM (same approach as Artifacts.test.tsx),
// but keep onValueChange wired so tests can still pick options as plain buttons.
jest.mock('@/components/ui/select', () => {
  const ReactActual = jest.requireActual<typeof import('react')>('react')
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
jest.mock('@/components/ui/tabs', () => {
  const ReactActual = jest.requireActual<typeof import('react')>('react')
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
// textarea keeps the body editable without any of that.
jest.mock('@/components/PromptMentionTextarea', () => ({
  PromptMentionTextarea: ({
    value,
    onChange,
  }: {
    value: string
    onChange: (v: string) => void
  }) => (
    <textarea
      data-testid="prompt-body-textarea"
      value={value}
      onChange={e => {
        onChange(e.target.value)
      }}
    />
  ),
}))

jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-preview">{content}</div>
  ),
}))

// Template loader dialog → a bare button that hands back a fixed template.
jest.mock('@/components/PromptTemplateLoader', () => ({
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

jest.mock('@/services/promptService', () => ({
  promptService: {
    getPrompt: jest.fn(),
    getPrompts: jest.fn(),
    createPrompt: jest.fn(),
    updatePrompt: jest.fn(),
    getPromptPlaceholders: jest.fn(),
    renderPrompt: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

jest.mock('@/hooks', () => {
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
    useAnalytics: () => ({ trackEvent }),
  }
})

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
    info: jest.fn(),
    warning: jest.fn(),
    message: jest.fn(),
  },
}))

import React from 'react'

import { useAnalytics } from '@/hooks'
import { toast } from '@/lib/toast'
import { projectService } from '@/services/projectService'
import { promptService } from '@/services/promptService'

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

const mockTemplatePrompt = buildPrompt({
  name: 'Great Template',
  slug: 'great-template',
  description: 'Template description',
  body: 'Template body',
})

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
    | string
    | { pathname: string; state?: unknown } = '/prompts/create'
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
  jest.clearAllMocks()
  ;(projectService.getProjects as jest.Mock).mockResolvedValue({
    projects: [projectAlpha],
  })
  ;(promptService.getPrompts as jest.Mock).mockResolvedValue({ prompts: [] })
  ;(promptService.createPrompt as jest.Mock).mockResolvedValue(buildPrompt())
  ;(promptService.updatePrompt as jest.Mock).mockResolvedValue(buildPrompt())
})

describe('PromptEditor — create mode', () => {
  it('renders an empty form and navigates back on Back', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(screen.getByText('Create new prompt')).toBeInTheDocument()
    expect(screen.getByTestId('prompt-name-input')).toHaveValue('')
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue('')
    expect(screen.getByText('Save as draft')).toBeInTheDocument()
    expect(promptService.getPrompt).not.toHaveBeenCalled()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {})
    })

    await user.click(screen.getByRole('button', { name: /back/i }))
    expect(mockNavigate).toHaveBeenCalledWith('/prompts')
  })

  it('auto-generates the slug from the name and creates the prompt with the typed payload', async () => {
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.type(screen.getByTestId('prompt-name-input'), 'My New Prompt!')
    // Auto-slug is shown next to the name field
    expect(await screen.findByText('my-new-prompt')).toBeInTheDocument()

    await user.type(
      screen.getByTestId('prompt-body-textarea'),
      'Review the code'
    )

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.createPrompt).toHaveBeenCalledWith('team-1', {
        name: 'My New Prompt!',
        slug: 'my-new-prompt',
        description: '',
        body: 'Review the code',
        status: 'draft',
        mcp_expose: false,
        // The single available project was auto-selected.
        project_id: 'p1',
        labels: [],
      })
    })
    expect(promptService.updatePrompt).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('Prompt created successfully')
    expect(mockNavigate).toHaveBeenCalledWith('/prompts/my-new-prompt')
  })

  it('surfaces validation errors and does not save an empty form', async () => {
    // Two projects → no auto-selection, so project_id is empty too.
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [projectAlpha, projectBeta],
    })
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.click(screen.getByTestId('prompt-save-button'))

    expect(toast.error).toHaveBeenCalledWith('Please fix the validation errors')
    expect(screen.getByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Slug is required')).toBeInTheDocument()
    expect(screen.getByText('Project is required')).toBeInTheDocument()
    expect(screen.getByText('Prompt content is required')).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('rejects a manually entered slug with invalid characters', async () => {
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.type(screen.getByTestId('prompt-name-input'), 'Named')
    await user.type(screen.getByTestId('prompt-body-textarea'), 'Body')

    const slugInput = screen.getByTestId('prompt-slug-input')
    await user.clear(slugInput)
    await user.type(slugInput, 'Bad Slug!')

    // Manual edit turns off auto-generation
    expect(
      screen.getByText(/Custom slug \(won't auto-update\)/)
    ).toBeInTheDocument()

    await user.click(screen.getByTestId('prompt-save-button'))

    expect(
      screen.getByText(
        'Slug must contain only lowercase letters, numbers, and hyphens'
      )
    ).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('carries settings-pane edits (status, MCP, labels, description) into the payload', async () => {
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.type(screen.getByTestId('prompt-name-input'), 'Configured')
    await user.type(screen.getByTestId('prompt-body-textarea'), 'Body')

    // Description with live character counter
    const description = screen.getByPlaceholderText(
      'Enter a brief description…'
    )
    await user.type(description, 'Short summary')
    expect(screen.getByText('13/200')).toBeInTheDocument()

    // Labels: add two, ignore a duplicate, then remove one
    const labelsInput = screen.getByPlaceholderText('Type and press Enter…')
    await user.type(labelsInput, 'alpha{Enter}')
    await user.type(labelsInput, 'beta{Enter}')
    await user.type(labelsInput, 'alpha{Enter}')
    expect(screen.getByText('2/10 labels')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove alpha' }))
    expect(screen.getByText('1/10 labels')).toBeInTheDocument()

    // Status → published flips the save label and reveals the MCP switch
    await user.click(screen.getByRole('button', { name: 'Published' }))
    expect(screen.getByText('Publish')).toBeInTheDocument()
    await user.click(screen.getByRole('switch'))

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.createPrompt).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({
          description: 'Short summary',
          labels: ['beta'],
          status: 'published',
          mcp_expose: true,
        })
      )
    })
  })

  it('rejects a description longer than 200 characters', async () => {
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.type(screen.getByTestId('prompt-name-input'), 'Named')
    await user.type(screen.getByTestId('prompt-body-textarea'), 'Body')
    // Bypass the textarea's maxLength the way a paste would
    fireEvent.change(
      screen.getByPlaceholderText('Enter a brief description…'),
      { target: { value: 'x'.repeat(201) } }
    )

    await user.click(screen.getByTestId('prompt-save-button'))

    expect(
      screen.getByText('Description cannot be longer than 200 characters')
    ).toBeInTheDocument()
    expect(promptService.createPrompt).not.toHaveBeenCalled()
  })

  it('prefills the form from navigation state', () => {
    renderEditor({
      pathname: '/prompts/create',
      state: { title: 'Source Prompt', body: 'Copied body', description: 'D' },
    })

    expect(screen.getByTestId('prompt-name-input')).toHaveValue(
      'Based on: Source Prompt'
    )
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
      'Copied body'
    )
  })

  it('loads a template into the form via the template loader', async () => {
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.click(screen.getByRole('button', { name: /load template/i }))
    await user.click(screen.getByTestId('template-loader-pick'))

    expect(screen.getByTestId('prompt-name-input')).toHaveValue(
      'Great Template (Copy)'
    )
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
      'Template body'
    )
    expect(toast.success).toHaveBeenCalledWith(
      'Template "Great Template" loaded'
    )
  })

  it('asks for confirmation before a template overwrites existing content', async () => {
    const confirmSpy = jest.spyOn(window, 'confirm').mockReturnValue(false)
    const user = userEvent.setup()
    renderEditor()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })

    await user.type(screen.getByTestId('prompt-body-textarea'), 'Precious')
    await user.click(screen.getByRole('button', { name: /load template/i }))
    await user.click(screen.getByTestId('template-loader-pick'))

    expect(confirmSpy).toHaveBeenCalled()
    // Declined → content untouched
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue('Precious')
    confirmSpy.mockRestore()
  })
})

describe('PromptEditor — edit mode', () => {
  it('prefills the form from the loaded prompt and updates on save', async () => {
    ;(promptService.getPrompt as jest.Mock).mockResolvedValue(buildPrompt())
    const user = userEvent.setup()
    renderEditor('/prompts/my-prompt/edit')

    expect(await screen.findByText('Edit prompt')).toBeInTheDocument()
    expect(promptService.getPrompt).toHaveBeenCalledWith('team-1', 'my-prompt')
    expect(screen.getByTestId('prompt-name-input')).toHaveValue('My Prompt')
    expect(screen.getByTestId('prompt-body-textarea')).toHaveValue(
      'Hello {{name}}'
    )
    expect(screen.getByTestId('prompt-slug-input')).toHaveValue('my-prompt')
    // Published prompt → the save button reads Publish
    expect(screen.getByText('Publish')).toBeInTheDocument()

    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(promptService.updatePrompt).toHaveBeenCalledWith(
        'team-1',
        'my-prompt',
        {
          name: 'My Prompt',
          slug: 'my-prompt',
          description: 'A description',
          body: 'Hello {{name}}',
          status: 'published',
          mcp_expose: true,
          labels: ['review'],
          project_id: 'p1',
        }
      )
    })
    expect(promptService.createPrompt).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('Prompt updated successfully')
    expect(mockNavigate).toHaveBeenCalledWith('/prompts/my-prompt')
  })

  it('opens the render view, loads placeholders, and shows the rendered output', async () => {
    ;(promptService.getPrompt as jest.Mock).mockResolvedValue(buildPrompt())
    ;(promptService.getPromptPlaceholders as jest.Mock).mockResolvedValue([
      'name',
    ])
    ;(promptService.renderPrompt as jest.Mock).mockResolvedValue({
      rendered_body: 'Hello Ada',
    })
    const { trackEvent } = useAnalytics()
    const user = userEvent.setup()
    renderEditor('/prompts/my-prompt/edit')

    await screen.findByText('Edit prompt')
    await user.click(screen.getByTestId('tab-trigger-render'))

    await waitFor(() => {
      expect(promptService.getPromptPlaceholders).toHaveBeenCalledWith(
        'team-1',
        'my-prompt'
      )
    })
    expect(trackEvent).toHaveBeenCalledWith({
      event: 'prompt_preview_viewed',
      properties: expect.objectContaining({
        prompt_id: 'my-prompt',
        preview_type: 'render',
      }),
    })

    const placeholderInput = await screen.findByPlaceholderText(
      'Enter value for {{name}}'
    )
    await user.type(placeholderInput, 'Ada')

    // Rendering is debounced (500ms) behind the placeholder edits
    await waitFor(
      () => {
        expect(promptService.renderPrompt).toHaveBeenCalledWith(
          'team-1',
          'my-prompt',
          { name: 'Ada' }
        )
      },
      { timeout: 3000 }
    )
    expect(await screen.findByText('Hello Ada')).toBeInTheDocument()
  })

  it('shows an error toast and navigates back when the prompt fails to load', async () => {
    ;(promptService.getPrompt as jest.Mock).mockRejectedValue(
      new Error('prompt not found')
    )
    renderEditor('/prompts/missing/edit')

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('prompt not found')
    })
    expect(mockNavigate).toHaveBeenCalledWith('/prompts')
  })

  it('surfaces a save failure without navigating away', async () => {
    ;(promptService.getPrompt as jest.Mock).mockResolvedValue(buildPrompt())
    ;(promptService.updatePrompt as jest.Mock).mockRejectedValue(
      new Error('validation failed upstream')
    )
    const user = userEvent.setup()
    renderEditor('/prompts/my-prompt/edit')

    await screen.findByText('Edit prompt')
    await user.click(screen.getByTestId('prompt-save-button'))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('validation failed upstream')
    })
    expect(mockNavigate).not.toHaveBeenCalledWith('/prompts/my-prompt')
  })
})
