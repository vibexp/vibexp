import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import { modelProviderService } from '@/services/modelProviderService'

import { ModelProviderDialog } from './ModelProviderDialog'

jest.mock('@/services/modelProviderService', () => ({
  // Keep any other exports real; only the service singleton is mocked so
  // validate-on-save can be asserted.
  ...jest.requireActual('@/services/modelProviderService'),
  modelProviderService: {
    validateModelProvider: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

// Radix Select relies on browser APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

const mockedValidate =
  modelProviderService.validateModelProvider as jest.MockedFunction<
    typeof modelProviderService.validateModelProvider
  >
const mockedToastError = toast.error as jest.MockedFunction<typeof toast.error>

const fillValidForm = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.type(
    screen.getByPlaceholderText('e.g., OpenAI GPT-4o'),
    'My Provider'
  )
  await user.type(
    screen.getByPlaceholderText('e.g., gpt-4o-mini'),
    'gpt-4o-mini'
  )
  await user.type(
    screen.getByPlaceholderText('https://api.openai.com/v1'),
    'https://api.openai.com/v1'
  )
  await user.type(screen.getByPlaceholderText('Enter API key'), 'sk-test')
}

const renderDialog = (onSubmit = jest.fn().mockResolvedValue(undefined)) => {
  render(
    <ModelProviderDialog
      teamId="team-1"
      open
      onOpenChange={jest.fn()}
      submitting={false}
      onSubmit={onSubmit}
    />
  )
  return onSubmit
}

beforeEach(() => {
  jest.clearAllMocks()
})

describe('ModelProviderDialog', () => {
  it('shows the model field and a masked (password) API key input', () => {
    renderDialog()
    expect(screen.getByPlaceholderText('e.g., gpt-4o-mini')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Enter API key')).toHaveAttribute(
      'type',
      'password'
    )
  })

  it('validates on save and submits when the provider is valid', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'gpt-4o-mini' })
      )
    })
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-4o-mini' })
    )
  })

  it('blocks submit and shows an error when validation fails', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({
      is_valid: false,
      message: 'Could not reach the provider',
    })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedToastError).toHaveBeenCalledWith(
        'Could not reach the provider',
        expect.anything()
      )
    })
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('prefills the Base URL when a preset is selected', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Groq' }))

    expect(
      screen.getByPlaceholderText('https://api.openai.com/v1')
    ).toHaveValue('https://api.groq.com/openai/v1')
  })

  const existingProvider = {
    id: 'p1',
    user_id: 'u1',
    name: 'Existing',
    provider_type: 'openai_compatible',
    model: 'gpt-4o-mini',
    is_default: false,
    base_url: 'https://api.openai.com/v1',
    configuration: '{}',
    has_api_key: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    version: 1,
  }

  it('skips validation on a name-only edit (identity unchanged)', async () => {
    const user = userEvent.setup()
    const onSubmit = jest.fn().mockResolvedValue(undefined)
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={jest.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const name = screen.getByPlaceholderText('e.g., OpenAI GPT-4o')
    await user.clear(name)
    await user.type(name, 'Renamed')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled()
    })
    expect(mockedValidate).not.toHaveBeenCalled()
  })

  it('validates when the model changes on edit', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = jest.fn().mockResolvedValue(undefined)
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={jest.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const model = screen.getByPlaceholderText('e.g., gpt-4o-mini')
    await user.clear(model)
    await user.type(model, 'gpt-4o')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'gpt-4o' })
      )
    })
    expect(onSubmit).toHaveBeenCalled()
  })
})
