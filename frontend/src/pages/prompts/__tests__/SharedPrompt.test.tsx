import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HelmetProvider } from 'react-helmet-async'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { SharedPromptResponse } from '@/services/promptShareService'

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

vi.mock('@/services/promptShareService', () => ({
  promptShareService: {
    getSharedPrompt: vi.fn(),
  },
}))

// SharedPrompt is a public page: it renders outside TeamContext/AuthContext,
// so neither is mocked here — mounting without them IS the no-auth path.
const mockTrackEvent = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useAnalytics', () => ({
  useAnalytics: () => ({ trackEvent: mockTrackEvent }),
}))

import { deriveSiteInfo, EMPTY_SITE_INFO } from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'
import { promptShareService } from '@/services/promptShareService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { SharedPrompt } from '../SharedPrompt'

function buildSharedResponse(
  overrides: Partial<SharedPromptResponse> = {}
): SharedPromptResponse {
  return {
    prompt: {
      lock: null,
      id: 'prompt-1',
      name: 'Code Review Template',
      slug: 'code-review-template',
      description: 'Template for conducting code reviews',
      body: 'Please review this code for: {{criteria}}',
      user_id: 'user-1',
      team_id: 'team-1',
      project_id: 'proj-1',
      project: null,
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
    vi.clearAllMocks()
  })

  it('shows a loading spinner while the share is being resolved', () => {
    ;(promptShareService.getSharedPrompt as Mock).mockImplementation(
      () => new Promise(() => undefined)
    )

    renderSharedPrompt()

    expect(screen.getByText('Loading shared prompt…')).toBeInTheDocument()
  })

  it('renders the shared prompt from the share service', async () => {
    ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(
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

  it('brands the header as VibeXP, linking to the product, when nothing is configured', async () => {
    ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(
      buildSharedResponse()
    )

    const { container } = renderSharedPrompt()

    await waitFor(() => {
      expect(screen.getByText('Code Review Template')).toBeInTheDocument()
    })
    const brand = container.querySelector('header a')
    // The VibeXP mark beside the printed name is decorative (#1386): the
    // lockup is announced as "VibeXP" once, by its text.
    expect(brand?.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
    expect(screen.queryByRole('img', { name: 'VibeXP' })).toBeNull()
    expect(brand).toHaveTextContent('VibeXP')
    expect(brand).toHaveAttribute(
      'href',
      'https://vibexp.io?utm_source=prompt_shared_page'
    )
    expect(container.innerHTML).not.toContain('example.com')
    // Unbranded: the product tagline shows, the bundled PNG is not used as
    // the header logo any more, and there is no "Powered by" credit (#1361).
    expect(screen.getByText('AI-powered prompt management')).toBeInTheDocument()
    expect(container.querySelector('header img')).toBeNull()
    expect(screen.queryByText(/powered by/i)).not.toBeInTheDocument()
    await waitFor(() => {
      expect(document.title).toBe(
        'Code Review Template | Shared Prompt | VibeXP'
      )
    })
    expect(
      document.head.querySelector('meta[property="og:image"]')
    ).toHaveAttribute('content', `${window.location.origin}/logo_rounded.png`)
  })

  it('uses the company name, site and logo from site info when configured', async () => {
    ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(
      buildSharedResponse()
    )

    const { container } = render(
      <SiteInfoContext.Provider
        value={deriveSiteInfo({
          ...EMPTY_SITE_INFO,
          company_name: 'Acme Inc.',
          website: 'https://acme.test/?ref=home',
          logo_url: 'https://cdn.acme.test/logo.png',
        })}
      >
        <HelmetProvider>
          <MemoryRouter initialEntries={['/shared/prompts/share-token-123']}>
            <Routes>
              <Route path="/shared/prompts/:token" element={<SharedPrompt />} />
            </Routes>
          </MemoryRouter>
        </HelmetProvider>
      </SiteInfoContext.Provider>
    )

    await waitFor(() => {
      expect(screen.getByText('Code Review Template')).toBeInTheDocument()
    })
    expect(screen.getByRole('img', { name: 'Acme Inc.' })).toHaveAttribute(
      'src',
      'https://cdn.acme.test/logo.png'
    )
    // The tagline is VibeXP's own, so it is not shown under a company's name;
    // VibeXP is credited instead (#1361).
    expect(
      screen.queryByText('AI-powered prompt management')
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /powered by vibexp/i })
    ).toHaveAttribute('href', 'https://vibexp.io')
    expect(container.innerHTML).not.toContain('logo_rounded.png')
    expect(container.querySelector('header a')).toHaveAttribute(
      'href',
      'https://acme.test/?ref=home'
    )
    await waitFor(() => {
      expect(document.title).toBe(
        'Code Review Template | Shared Prompt | Acme Inc.'
      )
    })
    expect(
      document.head.querySelector('meta[property="og:site_name"]')
    ).toHaveAttribute('content', 'Acme Inc.')
    expect(
      document.head.querySelector('meta[property="og:image"]')
    ).toHaveAttribute('content', 'https://cdn.acme.test/logo.png')
  })

  it('marks a restricted share with the Restricted badge', async () => {
    ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(
      buildSharedResponse({ share_type: 'restricted' })
    )

    renderSharedPrompt()

    await waitFor(() => {
      expect(screen.getByText('Restricted')).toBeInTheDocument()
    })
    expect(screen.queryByText('Public')).not.toBeInTheDocument()
  })

  it('copies the rendered body to the clipboard', async () => {
    ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(
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
      ;(promptShareService.getSharedPrompt as Mock).mockRejectedValue(
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
      ;(promptShareService.getSharedPrompt as Mock).mockRejectedValue(
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
      ;(promptShareService.getSharedPrompt as Mock).mockResolvedValue(null)

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
