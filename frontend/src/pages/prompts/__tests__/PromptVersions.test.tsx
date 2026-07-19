import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Prompt } from '@/services/promptService'
import type { Team } from '@/services/teamService'
import type { ResourceVersion } from '@/types/version'

jest.mock('@/services/promptService', () => ({
  promptService: {
    getPrompt: jest.fn(),
    getPromptVersions: jest.fn(),
    restorePromptVersion: jest.fn(),
  },
}))

// Mutable so tests choose the team-loading / no-team states.
const teamContextValue: {
  currentTeam: Team | null
  isLoading: boolean
} = {
  currentTeam: { id: 'team-1', name: 'Test Team' } as Team,
  isLoading: false,
}
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => teamContextValue,
}))

const showSuccess = jest.fn()
const handleError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess, showError: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError }),
}))

// jsdom gaps that Radix (dropdown / alert-dialog) relies on — same approach as
// features/version-history/__tests__/VersionHistoryPage.test.tsx.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

import { promptService } from '@/services/promptService'

import { PromptVersions } from '../PromptVersions'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    id: 'prompt-1',
    name: 'My Prompt',
    slug: 'my-prompt',
    description: 'A description',
    body: 'live body\nline two',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'p1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-06-12T12:00:00Z',
    version: 3,
    ...overrides,
  }
}

function snapshot(
  n: number,
  content: string,
  summary: string
): ResourceVersion {
  return {
    id: `v${String(n)}`,
    team_id: 'team-1',
    resource_type: 'prompt',
    resource_id: 'prompt-1',
    version_number: n,
    content,
    change_summary: summary,
    actor_type: 'human',
    created_by: 'user-1',
    author: {
      id: 'user-1',
      display_name: 'Shaharia',
      avatar_url: null,
      initials: 'SA',
    },
    created_at: '2026-06-12T10:00:00.000Z',
  }
}

function renderVersions(slug = 'my-prompt') {
  return render(
    <MemoryRouter initialEntries={[`/prompts/${slug}/versions`]}>
      <Routes>
        <Route path="/prompts/:slug/versions" element={<PromptVersions />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  jest.clearAllMocks()
  teamContextValue.currentTeam = { id: 'team-1', name: 'Test Team' } as Team
  teamContextValue.isLoading = false
  ;(promptService.getPrompt as jest.Mock).mockResolvedValue(buildPrompt())
  ;(promptService.getPromptVersions as jest.Mock).mockResolvedValue({
    versions: [
      snapshot(2, 'second body', 'Second edit'),
      snapshot(1, 'first body', 'Created the prompt'),
    ],
  })
  ;(promptService.restorePromptVersion as jest.Mock).mockResolvedValue(
    buildPrompt()
  )
})

describe('PromptVersions', () => {
  it('shows a loading state while the team context resolves', () => {
    teamContextValue.isLoading = true
    renderVersions()

    expect(screen.getByText('Loading version history…')).toBeInTheDocument()
    expect(promptService.getPromptVersions).not.toHaveBeenCalled()
  })

  it('explains when no team is available', () => {
    teamContextValue.currentTeam = null
    renderVersions()

    expect(
      screen.getAllByText('Version history unavailable').length
    ).toBeGreaterThan(0)
    expect(
      screen.getByText(
        'No team available. Please select or create a team first.'
      )
    ).toBeInTheDocument()
    expect(promptService.getPromptVersions).not.toHaveBeenCalled()
  })

  it('renders the version timeline from the prompt version history', async () => {
    renderVersions()

    expect(await screen.findByText('Second edit')).toBeInTheDocument()
    expect(screen.getByText('Created the prompt')).toBeInTheDocument()
    expect(screen.getByText('Current')).toBeInTheDocument()
    // Synthesized current row = max snapshot + 1
    expect(screen.getByText('Version 3')).toBeInTheDocument()
    expect(screen.getByText(/My Prompt · 3 versions/)).toBeInTheDocument()

    expect(promptService.getPrompt).toHaveBeenCalledWith('team-1', 'my-prompt')
    expect(promptService.getPromptVersions).toHaveBeenCalledWith(
      'team-1',
      'my-prompt'
    )
  })

  it('restores a version through the confirmation dialog and reloads', async () => {
    const user = userEvent.setup()
    renderVersions()

    await screen.findByText('Second edit')
    await user.click(screen.getByLabelText('Restore version 1'))

    const dialog = await screen.findByTestId('restore-version-dialog')
    expect(within(dialog).getByText(/Restore Version 1\?/)).toBeInTheDocument()

    await user.click(screen.getByTestId('confirm-restore-button'))

    await waitFor(() => {
      expect(promptService.restorePromptVersion).toHaveBeenCalledWith(
        'team-1',
        'my-prompt',
        1
      )
    })
    expect(showSuccess).toHaveBeenCalledWith(
      'Version restored successfully',
      'Success'
    )
    // The page reloads the history after a restore.
    await waitFor(() => {
      expect(promptService.getPromptVersions).toHaveBeenCalledTimes(2)
    })
  })

  it('shows the empty state when the prompt has no snapshots yet', async () => {
    ;(promptService.getPromptVersions as jest.Mock).mockResolvedValue({
      versions: [],
    })
    renderVersions()

    expect(
      await screen.findByText('No previous versions yet.')
    ).toBeInTheDocument()
  })

  it('surfaces a load failure with the error alert', async () => {
    ;(promptService.getPromptVersions as jest.Mock).mockRejectedValue(
      new Error('history unavailable')
    )
    renderVersions()

    expect(
      await screen.findByText('Version history unavailable')
    ).toBeInTheDocument()
    expect(screen.getByText('history unavailable')).toBeInTheDocument()
    expect(handleError).toHaveBeenCalledWith(
      expect.any(Error),
      'Failed to load version history'
    )
  })

  it('passes the route slug through to the prompt version source', async () => {
    ;(promptService.getPrompt as jest.Mock).mockResolvedValue(
      buildPrompt({ slug: 'my prompt+x' })
    )
    renderVersions('my prompt+x')

    await screen.findByText('Second edit')
    expect(promptService.getPromptVersions).toHaveBeenCalledWith(
      'team-1',
      'my prompt+x'
    )
  })
})
