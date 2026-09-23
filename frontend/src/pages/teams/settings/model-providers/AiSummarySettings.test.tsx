import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import {
  aiSummarySettingsService,
  type TeamAISummarySettings,
} from '@/services/aiSummarySettingsService'
import type { ModelProviderResponse } from '@/services/modelProviderService'
import type { Team } from '@/services/teamService'
import { ApiError } from '@/types/errors'

import { AiSummarySettings } from './AiSummarySettings'

// Stable handleError reference, like the real useCallback-backed hook, so the
// load callback is not recreated every render.
const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

// usePermissions reads the membership list from TeamContext.
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: null, teams: [] }),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/aiSummarySettingsService', () => ({
  aiSummarySettingsService: {
    getAISummarySettings: vi.fn(),
    updateAISummarySettings: vi.fn(),
    resetAISummarySettings: vi.fn(),
  },
}))

const service = aiSummarySettingsService as Mocked<
  typeof aiSummarySettingsService
>

// `permissions` is what usePermissions reads — deliberately NOT mocked, so the
// edit gate is exercised through the real hook.
const admin = {
  id: 'team-1',
  name: 'Team',
  permissions: ['team.settings.update'],
} as unknown as Team
const member = { ...admin, permissions: [] } as unknown as Team

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

const defaults = {
  enabled: false,
  model_provider_id: null,
  top_n: 5,
  style: 'balanced' as const,
  max_output_tokens: 800,
}

const instanceSettings: TeamAISummarySettings = {
  source: 'instance',
  values: defaults,
  instance_defaults: defaults,
  max_top_n: 10,
  max_output_tokens_ceiling: 4096,
  available: true,
}

const teamSettings: TeamAISummarySettings = {
  ...instanceSettings,
  source: 'team',
  values: { ...defaults, enabled: true, model_provider_id: 'provider-1' },
}

const renderCard = (team: Team = admin, reloadKey = 0) =>
  render(
    <AiSummarySettings
      team={team}
      providers={[provider]}
      reloadKey={reloadKey}
    />
  )

beforeEach(() => {
  vi.clearAllMocks()
  service.getAISummarySettings.mockResolvedValue(instanceSettings)
})

describe('AiSummarySettings', () => {
  it('renders the instance defaults with no reset action', async () => {
    renderCard()

    expect(
      await screen.findByText('Using instance defaults')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Results to read')).toHaveValue(5)
    expect(screen.getByLabelText('Response length (tokens)')).toHaveValue(800)
    expect(screen.getByLabelText('Style')).toHaveValue('balanced')
    expect(screen.getByLabelText('Provider')).toHaveValue('')
    expect(
      screen.getByRole('option', { name: 'OpenAI — gpt-4o-mini' })
    ).toBeInTheDocument()
    expect(screen.getByRole('switch')).not.toBeChecked()
    expect(
      screen.queryByRole('button', { name: /Reset to defaults/ })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    expect(service.getAISummarySettings).toHaveBeenCalledWith('team-1')
  })

  it('saves the whole profile and flips to the team indicator', async () => {
    const user = userEvent.setup()
    service.updateAISummarySettings.mockResolvedValue(teamSettings)
    renderCard()

    await screen.findByText('Using instance defaults')
    await user.click(screen.getByRole('switch'))
    await user.selectOptions(screen.getByLabelText('Provider'), 'provider-1')
    await user.selectOptions(screen.getByLabelText('Style'), 'detailed')
    const tokens = screen.getByLabelText('Response length (tokens)')
    await user.clear(tokens)
    await user.type(tokens, '400')
    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(service.updateAISummarySettings).toHaveBeenCalledWith('team-1', {
      enabled: true,
      model_provider_id: 'provider-1',
      top_n: 5,
      style: 'detailed',
      max_output_tokens: 400,
    })
    expect(
      await screen.findByText('Customized for this team')
    ).toBeInTheDocument()
    expect(screen.getByText('AI Summary settings saved.')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Reset to defaults/ })
    ).toBeInTheDocument()
  })

  it('resets to the instance defaults and hides the reset action', async () => {
    const user = userEvent.setup()
    service.getAISummarySettings
      .mockResolvedValueOnce(teamSettings)
      .mockResolvedValueOnce(instanceSettings)
    service.resetAISummarySettings.mockResolvedValue(undefined)
    renderCard()

    await screen.findByText('Customized for this team')
    expect(
      screen.getByText(/Reset would restore the instance defaults: off/)
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Reset to defaults/ }))

    expect(
      await screen.findByText('Using instance defaults')
    ).toBeInTheDocument()
    expect(service.resetAISummarySettings).toHaveBeenCalledWith('team-1')
    expect(service.getAISummarySettings).toHaveBeenCalledTimes(2)
    expect(
      screen.getByText('Reset to the instance defaults.')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Reset to defaults/ })
    ).not.toBeInTheDocument()
  })

  it('surfaces a failed reset', async () => {
    const user = userEvent.setup()
    service.getAISummarySettings.mockResolvedValue(teamSettings)
    service.resetAISummarySettings.mockRejectedValue(new Error('reset boom'))
    renderCard()

    await user.click(
      await screen.findByRole('button', { name: /Reset to defaults/ })
    )

    expect(await screen.findByText('reset boom')).toBeInTheDocument()
  })

  it('clamps results-to-read to the response max_top_n', async () => {
    const user = userEvent.setup()
    renderCard()

    const topN = await screen.findByLabelText('Results to read')
    expect(topN).toHaveAttribute('max', '10')
    await user.clear(topN)
    await user.type(topN, '42')
    expect(topN).toHaveValue(10)

    await user.clear(topN)
    await user.type(topN, '0')
    expect(topN).toHaveValue(1)
  })

  it('clamps response length to the response max_output_tokens_ceiling', async () => {
    const user = userEvent.setup()
    renderCard()

    const tokens = await screen.findByLabelText('Response length (tokens)')
    expect(tokens).toHaveAttribute('max', '4096')
    expect(screen.getByText(/\(1–4096\)/)).toBeInTheDocument()
    await user.clear(tokens)
    await user.type(tokens, '9999')
    expect(tokens).toHaveValue(4096)

    await user.clear(tokens)
    await user.type(tokens, '0')
    expect(tokens).toHaveValue(1)
  })

  it('blocks saving an invalid response length', async () => {
    const user = userEvent.setup()
    renderCard()

    const tokens = await screen.findByLabelText('Response length (tokens)')
    await user.clear(tokens)

    expect(
      screen.getByText(
        'Response length must be a whole number between 1 and 4096.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('shows the API message on a rejected save and keeps the edits', async () => {
    const user = userEvent.setup()
    service.updateAISummarySettings.mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'invalid_request',
        status: 400,
        detail: 'model_provider_id does not belong to this team',
        code: 'invalid_request',
      } as ConstructorParameters<typeof ApiError>[0])
    )
    renderCard()

    await screen.findByText('Using instance defaults')
    await user.selectOptions(screen.getByLabelText('Style'), 'concise')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(
      await screen.findByText('model_provider_id does not belong to this team')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Style')).toHaveValue('concise')
    expect(screen.getByText('Using instance defaults')).toBeInTheDocument()
  })

  it('shows a member the values read-only with no save or reset', async () => {
    service.getAISummarySettings.mockResolvedValue(teamSettings)
    renderCard(member)

    await screen.findByText('Customized for this team')
    expect(
      screen.getByText('Only team owners and admins can change these settings.')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Style')).toBeDisabled()
    expect(screen.getByLabelText('Results to read')).toBeDisabled()
    expect(screen.getByRole('switch')).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Save changes' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Reset to defaults/ })
    ).not.toBeInTheDocument()
  })

  it('explains the provider prerequisite instead of rendering inputs', async () => {
    service.getAISummarySettings.mockResolvedValue({
      ...instanceSettings,
      available: false,
    })
    renderCard()

    expect(
      await screen.findByTestId('ai-summary-unavailable')
    ).toHaveTextContent('AI Summary needs a model provider')
    expect(screen.queryByLabelText('Results to read')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Save changes' })
    ).not.toBeInTheDocument()
  })

  it('re-fetches when the page bumps reloadKey', async () => {
    service.getAISummarySettings
      .mockResolvedValueOnce({ ...instanceSettings, available: false })
      .mockResolvedValueOnce(instanceSettings)
    const { rerender } = renderCard()

    await screen.findByTestId('ai-summary-unavailable')
    rerender(
      <AiSummarySettings team={admin} providers={[provider]} reloadKey={1} />
    )

    expect(await screen.findByLabelText('Results to read')).toBeInTheDocument()
    expect(service.getAISummarySettings).toHaveBeenCalledTimes(2)
  })

  it('routes a load failure through the error handler', async () => {
    const failure = new Error('load boom')
    service.getAISummarySettings.mockRejectedValue(failure)
    renderCard()

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        failure,
        'Failed to load AI Summary settings'
      )
    })
    expect(
      await screen.findByTestId('ai-summary-load-failed')
    ).toBeInTheDocument()
  })
})
