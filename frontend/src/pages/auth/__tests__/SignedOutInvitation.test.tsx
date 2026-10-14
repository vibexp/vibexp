import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { authService } from '@/services/authService'
import { publicInvitationService } from '@/services/publicInvitationService'
import { ApiError } from '@/types/errors'
import { sessionStore } from '@/utils/storage'

const { login } = vi.hoisted(() => ({ login: vi.fn() }))

vi.mock('@/services/publicInvitationService', () => ({
  publicInvitationService: { preview: vi.fn(), register: vi.fn() },
}))
vi.mock('@/services/authService', () => ({
  authService: { getSignInMethods: vi.fn() },
}))
vi.mock('@/contexts/useAuth', () => ({ useAuth: () => ({ login }) }))

import { SignedOutInvitation } from '../invitation/SignedOutInvitation'

const previews = vi.mocked(publicInvitationService)
const auth = vi.mocked(authService)
const go = vi.fn()
const PASSPHRASE = 'a long invitee passphrase'

const PREVIEW = {
  team_name: 'Invite Team',
  inviter_name: 'Test Admin',
  email: 'grace@example.com',
  role: 'member' as const,
  expires_at: '2026-10-21T10:00:00Z',
  account_exists: false,
  password_registration: true,
}

const MOCK_SSO = { name: 'mock-sso', display_name: 'Mock SSO', type: 'oidc' }

function apiError(
  status: number,
  code: string,
  detail: string,
  field?: string
) {
  return new ApiError({
    type: 'about:blank',
    title: 'Refused',
    status,
    detail,
    code,
    request_id: 'r',
    timestamp: '2026-10-14T00:00:00Z',
    ...(field
      ? { validation_errors: [{ field, message: detail, code: 'too_short' }] }
      : {}),
  })
}

function renderView() {
  render(<SignedOutInvitation token="tok+1/a" go={go} />, {
    wrapper: MemoryRouter,
  })
  return userEvent.setup()
}

async function register(user: ReturnType<typeof userEvent.setup>) {
  const form = within(
    await screen.findByRole('form', { name: 'Create your account' })
  )
  await user.type(form.getByLabelText('Name (optional)'), 'Grace Hopper')
  await user.type(form.getByLabelText('Password'), PASSPHRASE)
  await user.type(form.getByLabelText('Repeat the password'), PASSPHRASE)
  await user.click(
    form.getByRole('button', { name: 'Create account and join' })
  )
  return form
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStore.clear()
  previews.preview.mockResolvedValue(PREVIEW)
  auth.getSignInMethods.mockResolvedValue({
    providers: [MOCK_SSO],
    password: { enabled: true, reset: 'email' },
  } as never)
})

describe('SignedOutInvitation', () => {
  it('shows the invitation and both ways to create the account', async () => {
    renderView()

    const card = await screen.findByTestId('invitation-preview')
    for (const text of [
      'Invite Team',
      'Test Admin',
      'grace@example.com',
      'member',
    ]) {
      expect(card).toHaveTextContent(text)
    }
    expect(previews.preview).toHaveBeenCalledWith('tok+1/a')
    expect(
      screen.getByRole('form', { name: 'Create your account' })
    ).toBeVisible()
    expect(screen.getByText(/At least 12 characters/)).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Continue with Mock SSO' })
    ).toBeVisible()
  })

  it('creates the account and lands in the team with the welcome handshake', async () => {
    previews.register.mockResolvedValue({
      user: { email: 'grace@example.com' },
      team_id: 'team-1',
      invitation_accepted: true,
    } as never)
    const user = renderView()
    await register(user)

    await waitFor(() => {
      expect(go).toHaveBeenCalledWith('/teams/team-1')
    })
    expect(previews.register).toHaveBeenCalledWith('tok+1/a', {
      password: PASSPHRASE,
      name: 'Grace Hopper',
    })
    expect(sessionStore.getJSON(STORAGE_KEYS.INVITATION_JUST_ACCEPTED)).toEqual(
      { team_id: 'team-1', team_name: 'Invite Team' }
    )
  })

  it('says the invitation is still waiting when the account was made but the join did not finish', async () => {
    previews.register.mockResolvedValue({
      user: { email: 'grace@example.com' },
      team_id: 'team-1',
      invitation_accepted: false,
    } as never)
    const user = renderView()
    await register(user)

    const waiting = await screen.findByTestId('invitation-waiting')
    expect(waiting).toHaveTextContent('Your account is ready')
    expect(waiting).toHaveTextContent('signed in as grace@example.com')
    expect(waiting).toHaveTextContent('still waiting')
    await user.click(
      screen.getByRole('button', { name: 'Open the invitation' })
    )
    expect(go).toHaveBeenCalledWith('/invitations/accept/tok%2B1%2Fa')
  })

  it('offers only sign-in when the account exists, and resumes the invitation after it', async () => {
    previews.preview.mockResolvedValue({
      ...PREVIEW,
      account_exists: true,
      password_registration: false,
    })
    const user = renderView()

    const signIn = await screen.findByTestId('invitation-sign-in')
    expect(screen.queryByRole('form')).toBeNull()
    await user.click(
      within(signIn).getByRole('button', { name: 'Sign in to accept' })
    )
    expect(sessionStore.get(STORAGE_KEYS.PENDING_INVITATION_TOKEN)).toBe(
      'tok+1/a'
    )
    expect(go).toHaveBeenCalledWith('/login')
  })

  it('switches to sign-in when the server finds an account after all', async () => {
    previews.register.mockRejectedValue(
      apiError(409, 'account_exists', 'An account already exists')
    )
    const user = renderView()
    await register(user)

    expect(await screen.findByTestId('invitation-sign-in')).toBeVisible()
  })

  it('offers only providers while password sign-in is off, and stashes the invitation before leaving', async () => {
    previews.preview.mockResolvedValue({
      ...PREVIEW,
      password_registration: false,
    })
    login.mockResolvedValue(undefined)
    const user = renderView()

    await screen.findByTestId('invitation-create')
    expect(screen.queryByRole('form')).toBeNull()
    await user.click(
      screen.getByRole('button', { name: 'Continue with Mock SSO' })
    )
    expect(sessionStore.get(STORAGE_KEYS.PENDING_INVITATION_TOKEN)).toBe(
      'tok+1/a'
    )
    expect(login).toHaveBeenCalledWith('mock-sso')
  })

  it('shows one state for any unusable link', async () => {
    previews.preview.mockRejectedValue(
      apiError(404, 'invitation_invalid', 'not valid')
    )
    renderView()

    const invalid = await screen.findByTestId('invitation-invalid')
    expect(invalid).toHaveTextContent('This invitation link is not valid')
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
    // A way on, and where the person is (#1473).
    expect(
      within(invalid).getByRole('link', { name: 'Go to sign in' })
    ).toHaveAttribute('href', '/login')
    expect(
      within(invalid).getByTestId('public-instance-name')
    ).toHaveTextContent('VibeXP')
  })

  it('offers to try again when the invitation could not be loaded', async () => {
    previews.preview
      .mockRejectedValueOnce(new Error('Network Error'))
      .mockResolvedValueOnce(PREVIEW)
    const user = renderView()

    await user.click(await screen.findByRole('button', { name: 'Try again' }))
    expect(await screen.findByTestId('invitation-preview')).toBeVisible()
  })

  it('puts a policy error on the password and a refusal on the form, keeping what was typed', async () => {
    previews.register.mockRejectedValueOnce(
      apiError(
        400,
        'VALIDATION_FAILED',
        'This password is too common',
        'password'
      )
    )
    const user = renderView()
    const form = await register(user)

    expect(await form.findByText('This password is too common')).toBeVisible()
    expect(form.getByLabelText('Password')).toHaveValue(PASSPHRASE)

    previews.register.mockRejectedValueOnce(
      apiError(
        403,
        'access_restricted',
        'Access to this instance is restricted.'
      )
    )
    await user.click(
      form.getByRole('button', { name: 'Create account and join' })
    )
    expect(
      await screen.findByTestId('invitation-register-error')
    ).toHaveTextContent('Access to this instance is restricted.')
    expect(go).not.toHaveBeenCalled()
  })

  it('catches a mismatch before any request', async () => {
    const user = renderView()
    const form = within(
      await screen.findByRole('form', { name: 'Create your account' })
    )
    await user.type(form.getByLabelText('Password'), PASSPHRASE)
    await user.type(
      form.getByLabelText('Repeat the password'),
      'something else entirely'
    )
    await user.click(
      form.getByRole('button', { name: 'Create account and join' })
    )
    expect(
      await form.findByText('The two passwords do not match.')
    ).toBeVisible()
    expect(previews.register).not.toHaveBeenCalled()
  })
})

describe('SignedOutInvitation: after review', () => {
  it('drops the provider stash once the account is made, on both outcomes', async () => {
    for (const accepted of [true, false]) {
      sessionStore.set(STORAGE_KEYS.PENDING_INVITATION_TOKEN, 'tok+1/a')
      previews.register.mockResolvedValueOnce({
        user: { email: 'grace@example.com' },
        team_id: 'team-1',
        invitation_accepted: accepted,
      } as never)
      const user = renderView()
      await register(user)
      await waitFor(() => {
        expect(
          sessionStore.get(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
        ).toBeNull()
      })
      cleanup()
    }
  })

  it('shows the invalid state when the invitation went away before the submit', async () => {
    previews.register.mockRejectedValue(
      apiError(404, 'invitation_invalid', 'not valid')
    )
    const user = renderView()
    await register(user)

    expect(await screen.findByTestId('invitation-invalid')).toBeVisible()
    expect(screen.queryByRole('form')).toBeNull()
  })

  it('treats an undecodable token as an invalid link', async () => {
    previews.preview.mockRejectedValue(
      apiError(400, 'BAD_REQUEST', 'Invalid request')
    )
    renderView()

    expect(await screen.findByTestId('invitation-invalid')).toHaveTextContent(
      'This invitation link is not valid'
    )
  })

  it('still shows the invitation when the provider list cannot be read', async () => {
    auth.getSignInMethods.mockRejectedValue(new Error('Network Error'))
    renderView()

    expect(
      await screen.findByRole('form', { name: 'Create your account' })
    ).toBeVisible()
    expect(screen.queryByTestId('provider-choices')).toBeNull()
  })

  it('says why when a provider sign-in cannot start, keeping the page', async () => {
    login.mockRejectedValue(new Error('Sign-in is unavailable'))
    const user = renderView()

    await user.click(
      await screen.findByRole('button', { name: 'Continue with Mock SSO' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Sign-in is unavailable'
    )
    expect(screen.getByTestId('invitation-preview')).toBeVisible()
  })

  it('has one page heading in every state', async () => {
    renderView()
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Team invitation' })
    ).toBeVisible()
    cleanup()

    previews.preview.mockRejectedValue(
      apiError(404, 'invitation_invalid', 'not valid')
    )
    renderView()
    await screen.findByTestId('invitation-invalid')
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
  })
})
