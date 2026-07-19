import { act, renderHook, waitFor } from '@testing-library/react'

import type { Prompt } from '@/services/promptService'
import type { Team } from '@/services/teamService'

jest.mock('@/services/promptService', () => ({
  promptService: {
    getPrompts: jest.fn(),
    createPrompt: jest.fn(),
    updatePrompt: jest.fn(),
    getPromptPlaceholders: jest.fn(),
    renderPrompt: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
    info: jest.fn(),
    warning: jest.fn(),
    message: jest.fn(),
  },
}))

import { toast } from '@/lib/toast'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import type { PromptFormData } from '../editor/types'
import { usePromptSave } from '../editor/usePromptSave'
import { useRenderPreview } from '../editor/useRenderPreview'
import { slugify, useSlugGeneration } from '../editor/useSlugGeneration'

const team = { id: 'team-1', name: 'Test Team' } as Team

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

const formData: PromptFormData = {
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
  jest.clearAllMocks()
})

describe('usePromptSave', () => {
  const trackEvent = jest.fn()

  it('refuses to save without a team', async () => {
    const { result } = renderHook(() =>
      usePromptSave({ teamId: undefined, prompt: null, trackEvent })
    )

    let saved: string | null = 'sentinel'
    await act(async () => {
      saved = await result.current.save(formData)
    })

    expect(saved).toBeNull()
    expect(toast.error).toHaveBeenCalledWith('No team selected')
    expect(promptService.createPrompt).not.toHaveBeenCalled()
    expect(promptService.updatePrompt).not.toHaveBeenCalled()
  })

  it('creates a new prompt with the full payload and tracks the event', async () => {
    ;(promptService.createPrompt as jest.Mock).mockResolvedValue(buildPrompt())
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: null, trackEvent })
    )

    let saved: string | null = null
    await act(async () => {
      saved = await result.current.save(formData)
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
    ;(promptService.updatePrompt as jest.Mock).mockResolvedValue(buildPrompt())
    const existing = buildPrompt({ slug: 'old-slug' })
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: existing, trackEvent })
    )

    let saved: string | null = null
    await act(async () => {
      saved = await result.current.save({ ...formData, slug: 'new-slug' })
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
    ;(promptService.createPrompt as jest.Mock).mockRejectedValue(
      new Error('slug already taken')
    )
    const { result } = renderHook(() =>
      usePromptSave({ teamId: 'team-1', prompt: null, trackEvent })
    )

    let saved: string | null = 'sentinel'
    await act(async () => {
      saved = await result.current.save(formData)
    })

    expect(saved).toBeNull()
    expect(toast.error).toHaveBeenCalledWith('slug already taken')
    expect(trackEvent).not.toHaveBeenCalled()
    expect(result.current.saving).toBe(false)
  })
})

describe('slugify', () => {
  it('normalizes names into URL-safe slugs', () => {
    expect(slugify('My Great Prompt')).toBe('my-great-prompt')
    expect(slugify('Hello, World! (v2)')).toBe('hello-world-v2')
    expect(slugify('  spaced   out  ')).toBe('spaced-out')
    expect(slugify('--already--dashed--')).toBe('already-dashed')
    expect(slugify('***')).toBe('')
  })
})

describe('useSlugGeneration', () => {
  it('returns an empty slug without a team or base', async () => {
    const { result } = renderHook(() => useSlugGeneration(null, undefined))
    await expect(result.current.generateUniqueSlug('base')).resolves.toBe('')

    const withTeam = renderHook(() => useSlugGeneration(team, undefined))
    await expect(withTeam.result.current.generateUniqueSlug('')).resolves.toBe(
      ''
    )
    expect(promptService.getPrompts).not.toHaveBeenCalled()
  })

  it('keeps the base slug when it is not taken', async () => {
    ;(promptService.getPrompts as jest.Mock).mockResolvedValue({
      prompts: [buildPrompt({ slug: 'other' })],
    })
    const { result } = renderHook(() => useSlugGeneration(team, undefined))

    await expect(result.current.generateUniqueSlug('fresh')).resolves.toBe(
      'fresh'
    )
    expect(promptService.getPrompts).toHaveBeenCalledWith('team-1', {
      limit: 1000,
    })
  })

  it('appends a random suffix when the slug collides', async () => {
    ;(promptService.getPrompts as jest.Mock).mockResolvedValue({
      prompts: [buildPrompt({ slug: 'taken' })],
    })
    const { result } = renderHook(() => useSlugGeneration(team, undefined))

    const slug = await result.current.generateUniqueSlug('taken')
    expect(slug).toMatch(/^taken-[a-z0-9]{4}$/)
  })

  it('ignores the prompt currently being edited when checking collisions', async () => {
    ;(promptService.getPrompts as jest.Mock).mockResolvedValue({
      prompts: [buildPrompt({ slug: 'my-prompt' })],
    })
    const { result } = renderHook(() => useSlugGeneration(team, 'my-prompt'))

    await expect(result.current.generateUniqueSlug('my-prompt')).resolves.toBe(
      'my-prompt'
    )
  })

  it('falls back to the base slug when the lookup fails', async () => {
    ;(promptService.getPrompts as jest.Mock).mockRejectedValue(
      new Error('network down')
    )
    const { result } = renderHook(() => useSlugGeneration(team, undefined))

    await expect(result.current.generateUniqueSlug('base')).resolves.toBe(
      'base'
    )
    expect(result.current.isCheckingSlug).toBe(false)
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
    ;(promptService.getPromptPlaceholders as jest.Mock).mockResolvedValue([
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
    ;(promptService.getPromptPlaceholders as jest.Mock).mockRejectedValue(
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
    ;(promptService.renderPrompt as jest.Mock).mockResolvedValue({
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
    ;(promptService.renderPrompt as jest.Mock).mockRejectedValue(
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
