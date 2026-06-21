import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn()
let mockPathname = '/'

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useLocation: () => ({
      pathname: mockPathname,
      search: '',
      hash: '',
      state: null,
      key: 'test',
    }),
  }
})

const sessionState = new Map<string, string>()

jest.mock('@/utils/storage', () => ({
  sessionStore: {
    get: (key: string) => sessionState.get(key) ?? null,
    remove: (key: string) => {
      sessionState.delete(key)
    },
  },
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { InvitationResumeAfterAuth } from './InvitationResumeAfterAuth'

const renderHere = () =>
  render(
    <MemoryRouter>
      <InvitationResumeAfterAuth />
    </MemoryRouter>
  )

describe('InvitationResumeAfterAuth', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    sessionState.clear()
    mockPathname = '/'
  })

  it('does nothing when no token is stashed', () => {
    renderHere()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('redirects to /invitations/accept/:token when a token is present', () => {
    sessionState.set('vx_pending_invitation_token', 'token-abc')

    renderHere()

    expect(mockNavigate).toHaveBeenCalledWith('/invitations/accept/token-abc', {
      replace: true,
    })
  })

  it('does not redirect when already on the accept page', () => {
    sessionState.set('vx_pending_invitation_token', 'token-abc')
    mockPathname = '/invitations/accept/token-abc'

    renderHere()

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('encodes tokens that contain special characters', () => {
    sessionState.set('vx_pending_invitation_token', 'a/b c')

    renderHere()

    expect(mockNavigate).toHaveBeenCalledWith('/invitations/accept/a%2Fb%20c', {
      replace: true,
    })
  })
})
