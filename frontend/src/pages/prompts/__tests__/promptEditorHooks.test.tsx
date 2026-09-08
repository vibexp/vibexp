import { act, renderHook, waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

import type { CreatePromptRequest, Prompt } from '@/services/promptService'

vi.mock('@/services/promptService', () => ({
  promptService: {
    getPrompts: vi.fn(),
    createPrompt: vi.fn(),
    updatePrompt: vi.fn(),
    getPromptPlaceholders: vi.fn(),
    renderPrompt: vi.fn(),
  },
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

import { toast } from '@/lib/toast'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { usePromptSave } from '../editor/usePromptSave'
import { useRenderPreview } from '../editor/useRenderPreview'

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

// Since #915 the hook takes the request body itself — `toPromptRequest` maps
// the generated form's parsed values to it — rather than a form-shaped object.
const payload: CreatePromptRequest = {
  name: 'My Prompt',
  slug: 'my-prompt',
  description: 'A description',
  body: 'Hello {{name}}',
  status: 'draft',
  mcp_expose: false,
  labels: ['review'],
  project_id: 'p1',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePromptSave', () => {
  const trackEvent = vi.fn()

  it('refuses to save without a team', async () => {
    const { result } = renderHook(() =>
      usePromptSave({ teamId: undefined, prompt: null, trackEvent })
    )

    let saved: string | null = 'sentinel'
    await act(async () => {
      saved = await result.current.save(payload)
    })

    expect(saved).toBeNull()
    expect(toast.error).toHaveBeenCalledWith('No team selected')
    expect(promptService.createPrompt).not.toHaveBeenCalled()
    expect(promptService.updatePrompt).not.toHaveBeenCalled()
  })

  it('creates a new prompt with the full payload and tracks the event', async () => {
    ;(promptService.createPrompt as Mock).mockResolvedValue(buildPrompt())
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: null, trackEvent })
    )

    let saved: string | null = null
    await act(async () => {
      saved = await result.current.save(payload)
    })

    expect(saved).toBe('my-prompt')
    expect(promptService.createPrompt).toHaveBeenCalledWith('team-1', {
      name: 'My Prompt',
      slug: 'my-prompt',
      description: 'A description',
      body: 'Hello {{name}}',
      status: 'draft',
      mcp_expose: false,
      labels: ['review'],
      project_id: 'p1',
    })
    expect(trackEvent).toHaveBeenCalledWith({
      event: ANALYTICS_EVENTS.PROMPT_CREATED,
      properties: {
        prompt_id: 'my-prompt',
        prompt_title: 'My Prompt',
        prompt_type: 'draft',
        action_context: 'create',
      },
    })
    expect(toast.success).toHaveBeenCalledWith('Prompt created successfully')
  })

  it('updates an existing prompt addressed by its current slug', async () => {
    ;(promptService.updatePrompt as Mock).mockResolvedValue(buildPrompt())
    const existing = buildPrompt({ slug: 'old-slug' })
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: existing, trackEvent })
    )

    let saved: string | null = null
    await act(async () => {
      saved = await result.current.save({ ...payload, slug: 'new-slug' })
    })

    // Returns the (possibly renamed) slug from the form…
    expect(saved).toBe('new-slug')
    // …but addresses the update by the prompt's existing slug.
    expect(promptService.updatePrompt).toHaveBeenCalledWith(
      'team-1',
      'old-slug',
      expect.objectContaining({ slug: 'new-slug' })
    )
    expect(promptService.createPrompt).not.toHaveBeenCalled()
    expect(trackEvent).toHaveBeenCalledWith(
      expect.objectContaining({ event: ANALYTICS_EVENTS.PROMPT_UPDATED })
    )
    expect(toast.success).toHaveBeenCalledWith('Prompt updated successfully')
  })

  it('reports failures and resets the saving flag', async () => {
    ;(promptService.createPrompt as Mock).mockRejectedValue(
      new Error('slug already taken')
    )
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: null, trackEvent })
    )

    let saved: string | null = 'sentinel'
    await act(async () => {
      saved = await result.current.save(payload)
    })

    expect(saved).toBeNull()
    expect(toast.error).toHaveBeenCalledWith('slug already taken')
    expect(trackEvent).not.toHaveBeenCalled()
    expect(result.current.saving).toBe(false)
  })
})

interface RenderPreviewProps {
  teamId: string | undefined
  prompt: Prompt | null
  view: 'write' | 'preview' | 'render'
  isEditing: boolean
}

describe('useRenderPreview', () => {
  const editingProps: RenderPreviewProps = {
    teamId: 'team-1',
    prompt: buildPrompt(),
    view: 'write',
    isEditing: true,
  }

  function renderPreviewHook(initialProps: RenderPreviewProps) {
    return renderHook(props => useRenderPreview(props), { initialProps })
  }

  it('loads placeholders for a saved prompt and seeds their values', async () => {
    ;(promptService.getPromptPlaceholders as Mock).mockResolvedValue([
      'name',
      'tone',
    ])
    const { result } = renderPreviewHook(editingProps)

    await act(async () => {
      await result.current.fetchAllPlaceholders()
    })

    expect(promptService.getPromptPlaceholders).toHaveBeenCalledWith(
      'team-1',
      'my-prompt'
    )
    expect(result.current.allPlaceholders).toEqual(['name', 'tone'])
    await waitFor(() => {
      expect(result.current.placeholderValues).toEqual({ name: '', tone: '' })
    })
  })

  it('does not fetch placeholders for an unsaved prompt', async () => {
    const { result } = renderPreviewHook({
      ...editingProps,
      prompt: null,
      isEditing: false,
    })

    await act(async () => {
      await result.current.fetchAllPlaceholders()
    })

    expect(promptService.getPromptPlaceholders).not.toHaveBeenCalled()
  })

  it('falls back to no placeholders when the lookup fails', async () => {
    ;(promptService.getPromptPlaceholders as Mock).mockRejectedValue(
      new Error('boom')
    )
    const { result } = renderPreviewHook(editingProps)

    await act(async () => {
      await result.current.fetchAllPlaceholders()
    })

    expect(result.current.allPlaceholders).toEqual([])
    expect(result.current.isLoadingPlaceholders).toBe(false)
  })

  it('renders the prompt (debounced) when the render view opens', async () => {
    ;(promptService.renderPrompt as Mock).mockResolvedValue({
      rendered_body: 'Hello Ada',
    })
    const { result, rerender } = renderPreviewHook(editingProps)

    act(() => {
      result.current.setPlaceholderValue('name', 'Ada')
    })
    rerender({ ...editingProps, view: 'render' })

    await waitFor(() => {
      expect(promptService.renderPrompt).toHaveBeenCalledWith(
        'team-1',
        'my-prompt',
        { name: 'Ada' }
      )
    })
    await waitFor(() => {
      expect(result.current.renderedBody).toBe('Hello Ada')
    })
    expect(result.current.renderError).toBeNull()
  })

  it('surfaces render failures and clears the previous output', async () => {
    ;(promptService.renderPrompt as Mock).mockRejectedValue(
      new Error('placeholder missing')
    )
    const { result, rerender } = renderPreviewHook(editingProps)

    rerender({ ...editingProps, view: 'render' })

    await waitFor(() => {
      expect(result.current.renderError).toBe('placeholder missing')
    })
    expect(result.current.renderedBody).toBe('')
    expect(result.current.isRendering).toBe(false)
  })

  it('refuses to render without a team even for a saved prompt', async () => {
    const { result, rerender } = renderPreviewHook({
      ...editingProps,
      teamId: undefined,
    })

    rerender({ ...editingProps, teamId: undefined, view: 'render' })

    await waitFor(() => {
      expect(result.current.renderError).toBe(
        'Cannot render unsaved prompt. Please save the prompt first.'
      )
    })
    expect(promptService.renderPrompt).not.toHaveBeenCalled()
  })
})
