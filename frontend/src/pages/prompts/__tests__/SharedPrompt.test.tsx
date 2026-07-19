import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HelmetProvider } from 'react-helmet-async'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { SharedPromptResponse } from '@/services/promptShareService'

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

jest.mock('@/services/promptShareService', () => ({
  promptShareService: {
    getSharedPrompt: jest.fn(),
  },
}))

// SharedPrompt is a public page: it renders outside TeamContext/AuthContext,
// so neither is mocked here — mounting without them IS the no-auth path.
const mockTrackEvent = jest.fn()
jest.mock('@/hooks/useAnalytics', () => ({
  useAnalytics: () => ({ trackEvent: mockTrackEvent }),
}))

import { promptShareService } from '@/services/promptShareService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { SharedPrompt } from '../SharedPrompt'

function buildSharedResponse(
  overrides: Partial<SharedPromptResponse> = {}
): SharedPromptResponse {
  return {
    prompt: {
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
      is_shared: true,
      labels: null,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-02T00:00:00Z',
      version: 1,
    },
    share_type: 'public',
    rendered_body: 'Please review this code for: {{criteria}}',
    ...overrides,
  }
}

function renderSharedPrompt(token = 'share-token-123') {
  return render(
    <HelmetProvider>
      <MemoryRouter initialEntries={[`/shared/prompts/${token}`]}>
        <Routes>
          <Route path="/shared/prompts/:token" element={<SharedPrompt />} />
          <Route
            path="/"
            element={<div data-testid="home-probe">Homepage probe</div>}
          />
        </Routes>
      </MemoryRouter>
    </HelmetProvider>
  )
}

describe('SharedPrompt page (public, no auth)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('shows a loading spinner while the share is being resolved', () => {
    ;(promptShareService.getSharedPrompt as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )

    renderSharedPrompt()

    expect(screen.getByText('Loading shared prompt…')).toBeInTheDocument()
  })

  it('renders the shared prompt from the share service', async () => {
    ;(promptShareService.getSharedPrompt as jest.Mock).mockResolvedValue(
      buildSharedResponse()
    )

    renderSharedPrompt()

    await waitFor(() => {
      expect(screen.getByText('Code Review Template')).toBeInTheDocument()
    })
    expect(promptShareService.getSharedPrompt).toHaveBeenCalledWith(
      'share-token-123'
    )
    expect(
      screen.getByText('Template for conducting code reviews')
    ).toBeInTheDocument()
    expect(screen.getByText('Shared prompt')).toBeInTheDocument()
    expect(screen.getByText('Public')).toBeInTheDocument()
    expect(screen.getByTestId('markdown-renderer')).toHaveTextContent(
      'Please review this code for: {{criteria}}'
    )
    expect(mockTrackEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        event: ANALYTICS_EVENTS.SHARED_PROMPT_VIEWED,
      })
    )
  })

  it('marks a restricted share with the Restricted badge', async () => {
    ;(promptShareService.getSharedPrompt as jest.Mock).mockResolvedValue(
      buildSharedResponse({ share_type: 'restricted' })
    )

    renderSharedPrompt()

    await waitFor(() => {
      expect(screen.getByText('Restricted')).toBeInTheDocument()
    })
    expect(screen.queryByText('Public')).not.toBeInTheDocument()
  })

  it('copies the rendered body to the clipboard', async () => {
    ;(promptShareService.getSharedPrompt as jest.Mock).mockResolvedValue(
      buildSharedResponse()
    )

    renderSharedPrompt()

    // userEvent.setup installs a clipboard stub; read it back to observe the write.
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /Copy/ }))

    expect(screen.getByText('Copied')).toBeInTheDocument()
    expect(await navigator.clipboard.readText()).toBe(
      'Please review this code for: {{criteria}}'
    )
  })

  describe('invalid token / error state', () => {
    it('shows the error card with the service message and tracks the failure', async () => {
      ;(promptShareService.getSharedPrompt as jest.Mock).mockRejectedValue(
        new Error('Share link expired')
      )

      renderSharedPrompt('expired-token')

      await waitFor(() => {
        expect(screen.getByText('Unable to load prompt')).toBeInTheDocument()
      })
      expect(screen.getByText('Share link expired')).toBeInTheDocument()
      expect(mockTrackEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          event: ANALYTICS_EVENTS.SHARED_PROMPT_ERROR,
          properties: expect.objectContaining({
            share_token: 'expired-token',
            error_message: 'Share link expired',
          }),
        })
      )
    })

    it('navigates to the homepage from the error card', async () => {
      ;(promptShareService.getSharedPrompt as jest.Mock).mockRejectedValue(
        new Error('Share link expired')
      )

      renderSharedPrompt()

      const user = userEvent.setup()
      await user.click(
        await screen.findByRole('button', { name: 'Go to homepage' })
      )

      expect(screen.getByTestId('home-probe')).toBeInTheDocument()
    })

    it('surfaces the error card instead of crashing when the service resolves with no data', async () => {
      // A null payload is unexpected (spec says an object); the page must fail
      // into its error card, not white-screen — the #121 drift failure class.
      ;(promptShareService.getSharedPrompt as jest.Mock).mockResolvedValue(null)

      renderSharedPrompt()

      await waitFor(() => {
        expect(screen.getByText('Unable to load prompt')).toBeInTheDocument()
      })
      expect(
        screen.getByRole('button', { name: 'Go to homepage' })
      ).toBeInTheDocument()
    })
  })
})
