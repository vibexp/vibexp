import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import type { ModelProviderResponse } from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'

import { ModelProviders } from './ModelProviders'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// Stable handleError reference (like the real useCallback-backed hook) so
// loadProviders isn't recreated every render — an unstable one loops the mount
// effect and remounts the section under test.
jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return { useErrorHandler: () => ({ handleError }) }
})

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

jest.mock('@/services/modelProviderService', () => ({
  ...jest.requireActual('@/services/modelProviderService'),
  modelProviderService: {
    getModelProviders: jest.fn(),
    deleteModelProvider: jest.fn(),
  },
}))

const service = modelProviderService as jest.Mocked<typeof modelProviderService>
const mockedToast = toast as jest.Mocked<typeof toast>

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

beforeEach(() => {
  jest.clearAllMocks()
  mockUseTeam.mockReturnValue({ currentTeam: { id: 'team-1', name: 'Team' } })
  service.getModelProviders.mockResolvedValue([provider])
  service.deleteModelProvider.mockResolvedValue(undefined)
})

describe('ModelProviders', () => {
  it('lists a team’s providers with name, model, and the API-key badge', async () => {
    render(<ModelProviders />)

    expect(await screen.findByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()
    expect(screen.getByText('Default')).toBeInTheDocument()
    expect(screen.getByText('Set')).toBeInTheDocument()
    expect(service.getModelProviders).toHaveBeenCalledWith('team-1')
  })

  it('renders the empty state when there are no providers', async () => {
    service.getModelProviders.mockResolvedValue([])

    render(<ModelProviders />)

    expect(
      await screen.findByText('No model providers yet')
    ).toBeInTheDocument()
  })

  it('deletes a provider after confirmation and refetches', async () => {
    const user = userEvent.setup()
    render(<ModelProviders />)

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
