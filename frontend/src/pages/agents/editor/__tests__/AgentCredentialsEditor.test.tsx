import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import { AgentCredentialsEditor } from '../AgentCredentialsEditor'

const mockUpdate = jest.fn()
jest.mock('@/services/agentService', () => ({
  agentService: {
    updateAgentCredentials: (...args: unknown[]) => mockUpdate(...args),
  },
}))
jest.mock('@/lib/toast', () => ({
  toast: { error: jest.fn(), success: jest.fn() },
}))

const schemes = {
  api_key: { type: 'apiKey' },
  oauth_token: { type: 'oauth2' },
}

describe('AgentCredentialsEditor', () => {
  beforeEach(() => {
    mockUpdate.mockReset()
    mockUpdate.mockResolvedValue(undefined)
  })

  it('renders an input for supported schemes and a note for unsupported ones', () => {
    render(
      <AgentCredentialsEditor
        agentId="a1"
        teamId="t1"
        securitySchemes={schemes}
      />
    )

    // apiKey scheme → an input.
    expect(screen.getByPlaceholderText('Enter api_key')).toBeInTheDocument()
    // oauth2 scheme → an unsupported note, no input.
    expect(screen.getByText(/not supported/i)).toBeInTheDocument()
    expect(
      screen.queryByPlaceholderText('Enter oauth_token')
    ).not.toBeInTheDocument()
  })

  it('sends only supported credentials, with their real scheme type', async () => {
    render(
      <AgentCredentialsEditor
        agentId="a1"
        teamId="t1"
        securitySchemes={schemes}
      />
    )

    fireEvent.change(screen.getByPlaceholderText('Enter api_key'), {
      target: { value: 'sk-123' },
    })
    fireEvent.click(screen.getByRole('button', { name: /update credentials/i }))

    await waitFor(() => {
      expect(mockUpdate).toHaveBeenCalledWith('t1', 'a1', {
        api_key: { type: 'apiKey', value: 'sk-123' },
      })
    })
  })

  it('renders nothing when there are no security schemes', () => {
    const { container } = render(
      <AgentCredentialsEditor agentId="a1" teamId="t1" securitySchemes={{}} />
    )
    expect(container).toBeEmptyDOMElement()
  })
})
