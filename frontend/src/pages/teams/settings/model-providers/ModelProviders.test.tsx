import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import { toast } from '@/lib/toast'
import type { ModelProviderResponse } from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'
import type { Team } from '@/services/teamService'

import { ModelProviders } from './ModelProviders'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// Stable handleError reference (like the real useCallback-backed hook) so
// loadProviders isn't recreated every render — an unstable one loops the mount
// effect and remounts the section under test.
vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
  return { useErrorHandler: () => ({ handleError }) }
})

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/modelProviderService', async () => ({
  ...(await vi.importActual('@/services/modelProviderService')),
  modelProviderService: {
    getModelProviders: vi.fn(),
    deleteModelProvider: vi.fn(),
    copyModelProviderFromTeam: vi.fn(),
  },
}))

const service = modelProviderService as Mocked<typeof modelProviderService>
const mockedToast = toast as Mocked<typeof toast>

const provider: ModelProviderResponse = {
  id: 'provider-1',
  user_id: 'user-1',
  name: 'OpenAI',
  provider_type: 'openai_compatible',
  model: 'gpt-4o-mini',
  is_default: true,
  base_url: 'https://api.openai.com/v1',
  configuration: '{}',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  version: 1,
  has_api_key: true,
}

// The team TeamScopeLayout resolved from the URL. Module-stable so a fresh
// identity per render cannot loop an effect keyed on it. `permissions` is what
// usePermissions reads — deliberately NOT mocked, so the copy gate is
// exercised through the real hook against these fixtures.
const urlTeam = {
  id: 'team-1',
  name: 'Team',
  permissions: ['team.update'],
} as unknown as Team

const sourceTeam = {
  id: 'team-2',
  name: 'Platform Team',
  permissions: ['team.update'],
} as unknown as Team

const sourceProvider: ModelProviderResponse = {
  ...provider,
  id: 'provider-9',
  name: 'Shared OpenAI',
  model: 'gpt-4o',
  is_default: true,
}

const renderPage = (team: Team = urlTeam) =>
  render(<ModelProviders team={team} />)

// cmdk (inside the copy dialog's popover) needs APIs jsdom does not provide.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: urlTeam,
    teams: [urlTeam, sourceTeam],
  })
  service.getModelProviders.mockResolvedValue([provider])
  service.deleteModelProvider.mockResolvedValue(undefined)
  service.copyModelProviderFromTeam.mockResolvedValue({
    ...sourceProvider,
    id: 'provider-copy',
    is_default: false,
  })
})

describe('ModelProviders', () => {
  it('lists a team’s providers with name, model, and the API-key badge', async () => {
    renderPage()

    expect(await screen.findByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()
    expect(screen.getByText('Default')).toBeInTheDocument()
    expect(screen.getByText('Set')).toBeInTheDocument()
    expect(service.getModelProviders).toHaveBeenCalledWith('team-1')
  })

  it('renders the empty state when there are no providers', async () => {
    service.getModelProviders.mockResolvedValue([])

    renderPage()

    expect(
      await screen.findByText('No model providers yet')
    ).toBeInTheDocument()
  })

  it('deletes a provider after confirmation and refetches', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('OpenAI')
    // The row action is the only "Delete" button until the confirm dialog opens.
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    // Confirm dialog appears; confirm the delete from within it (the row button
    // and the confirm button share the "Delete" name).
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(service.deleteModelProvider).toHaveBeenCalledWith(
        'team-1',
        'provider-1'
      )
    })
    expect(mockedToast.success).toHaveBeenCalledWith('Provider deleted')
    // Mount fetch + post-delete refetch.
    await waitFor(() => {
      expect(service.getModelProviders).toHaveBeenCalledTimes(2)
    })
  })
})

// ---------------------------------------------------------------------------
// #834 — copy a provider in from another team
// ---------------------------------------------------------------------------

describe('copy from another team (#834)', () => {
  // Walks the two-step flow up to the pre-filled provider dialog: pick the
  // source team, then the one provider the copy will move.
  const openCopyFlow = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByTestId('copy-model-provider-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Platform Team'))
    const radio = await screen.findByTestId('copy-source-provider')
    await user.click(radio)
    await user.click(screen.getByTestId('confirm-copy-from-team-button'))
  }

  beforeEach(() => {
    service.getModelProviders.mockImplementation((id: string) =>
      Promise.resolve(id === 'team-2' ? [sourceProvider] : [provider])
    )
  })

  it('offers the action beside "Add provider" and in the empty state', async () => {
    service.getModelProviders.mockResolvedValue([])
    renderPage()

    expect(
      await screen.findByTestId('copy-model-provider-button')
    ).toBeInTheDocument()
    expect(
      await screen.findByTestId('copy-model-provider-button-empty')
    ).toBeInTheDocument()
  })

  it('hides the action without team.update on this team', async () => {
    const noPerms = {
      id: 'team-1',
      name: 'Team',
      permissions: [],
    } as unknown as Team
    mockUseTeam.mockReturnValue({
      currentTeam: noPerms,
      teams: [noPerms, sourceTeam],
    })

    renderPage(noPerms)

    await screen.findByText('OpenAI')
    expect(screen.queryByTestId('copy-model-provider-button')).toBeNull()
  })

  it('hides the action when no other team grants team.update on the source', async () => {
    const readOnlyOther = {
      id: 'team-3',
      name: 'Read Only',
      permissions: [],
    } as unknown as Team
    mockUseTeam.mockReturnValue({
      currentTeam: urlTeam,
      teams: [urlTeam, readOnlyOther],
    })

    renderPage()

    await screen.findByText('OpenAI')
    expect(screen.queryByTestId('copy-model-provider-button')).toBeNull()
  })

  it('lists the source team’s providers once a source team is picked', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await user.click(screen.getByTestId('copy-model-provider-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Platform Team'))

    expect(await screen.findByText('Shared OpenAI')).toBeInTheDocument()
    expect(service.getModelProviders).toHaveBeenCalledWith('team-2')
    // Nothing picked yet, so the copy is not confirmable.
    expect(screen.getByTestId('confirm-copy-from-team-button')).toBeDisabled()
  })

  it('copies the chosen provider and reloads the list', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)

    // The provider dialog opened in copy mode, pre-filled from the source.
    expect(await screen.findByText('Copy model provider')).toBeInTheDocument()
    expect(screen.getByTestId('copy-api-key-field')).toHaveValue(
      'Will be copied from Platform Team'
    )

    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(service.copyModelProviderFromTeam).toHaveBeenCalledWith('team-1', {
        source_team_id: 'team-2',
        source_provider_id: 'provider-9',
        name: 'Shared OpenAI',
        provider_type: 'openai_compatible',
        model: 'gpt-4o',
        base_url: 'https://api.openai.com/v1',
      })
    })
    expect(mockedToast.success).toHaveBeenCalledWith(
      'Provider copied from Platform Team'
    )
    // Destination reload after the copy, so the new row (has_api_key: true)
    // comes from the server rather than being synthesized here.
    await waitFor(() => {
      expect(service.getModelProviders).toHaveBeenCalledWith('team-1')
      expect(
        service.getModelProviders.mock.calls.filter(c => c[0] === 'team-1')
      ).toHaveLength(2)
    })
  })

  it('sends an edited name as an override rather than the source name', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)

    const name = await screen.findByPlaceholderText('e.g., OpenAI GPT-4o')
    await user.clear(name)
    await user.type(name, 'Shared OpenAI (staging)')
    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(service.copyModelProviderFromTeam).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ name: 'Shared OpenAI (staging)' })
      )
    })
  })
})

// ---------------------------------------------------------------------------
// #584 — cold deep-link: act on the URL's team, never the ambient one
// ---------------------------------------------------------------------------
// React fires child effects before parent effects, so on a cold deep-link to
// /teams/B/... this page's load effect runs BEFORE TeamScopeLayout's
// setCurrentTeam sync and the ambient team is still A. These tests pin that the
// page reads the team from its prop by driving the two apart.

describe('cold deep-link (#584)', () => {
  const urlTeamB = { id: 'team-b', name: 'Team B' } as unknown as Team

  it('fetches the URL team, never the ambient one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A' },
      teams: [urlTeamB, sourceTeam],
    })

    renderPage(urlTeamB)

    await waitFor(() => {
      expect(service.getModelProviders).toHaveBeenCalledWith('team-b')
    })
    expect(service.getModelProviders).not.toHaveBeenCalledWith('team-a')
  })

  it('deletes against the URL team', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A' },
      teams: [urlTeamB, sourceTeam],
    })
    const user = userEvent.setup()

    renderPage(urlTeamB)
    await screen.findByText('OpenAI')

    // Same two-step selection as the existing delete test: the row action is
    // the only "Delete" button until the confirm dialog opens.
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(service.deleteModelProvider).toHaveBeenCalledWith(
        'team-b',
        'provider-1'
      )
    })
  })
})
