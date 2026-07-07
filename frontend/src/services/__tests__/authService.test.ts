import type { User } from '../authService'

const mockGeneratedClient = { GET: jest.fn(), POST: jest.fn() }

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { authService } from '../authService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })
const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2026-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const mockUser: User = {
  id: 'user-123',
  google_id: 'google-456',
  email: 'test@example.com',
  name: 'Test User',
  avatar_url: 'https://example.com/avatar.jpg',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  onboarding_completed: true,
  subscription_status: 'active',
  version: 1,
}

describe('AuthService (cookie-based auth)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getProviders unwraps the providers array', async () => {
    const providers = [{ name: 'google', display_name: 'Google' }]
    mockGeneratedClient.GET.mockReturnValue(success({ providers }))

    const result = await authService.getProviders()

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/auth/providers',
      {}
    )
    expect(result).toEqual(providers)
  })

  it('getLoginUrl returns the url and forwards the provider query', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ url: 'https://idp.example.com/authorize' })
    )

    const url = await authService.getLoginUrl('github')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith('/api/v1/auth/login', {
      params: { query: { provider: 'github' } },
    })
    expect(url).toBe('https://idp.example.com/authorize')
  })

  it('getCurrentUser returns the user from /auth/me', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(mockUser))

    expect(await authService.getCurrentUser()).toEqual(mockUser)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith('/api/v1/auth/me', {})
  })

  it('logout posts and resolves void', async () => {
    mockGeneratedClient.POST.mockReturnValue(success({ message: 'logged out' }))

    await expect(authService.logout()).resolves.toBeUndefined()
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/auth/logout',
      {}
    )
  })

  it('markOnboardingComplete returns the user', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(mockUser))

    expect(await authService.markOnboardingComplete()).toEqual(mockUser)
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/user/onboarding/complete',
      {}
    )
  })

  it('devLogin posts the email with the default name', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(mockUser))

    await authService.devLogin('dev@example.com')

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/auth/dev/login',
      { body: { email: 'dev@example.com', name: 'Dev User' } }
    )
  })

  it('throws ApiError with the backend detail on a 401', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      problem(401, 'Authentication required', 'AUTH_REQUIRED')
    )

    await expect(authService.getCurrentUser()).rejects.toThrow(
      'Authentication required'
    )
  })
})
