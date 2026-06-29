import type {
  OAuthConsentAttachResponse,
  OAuthConsentDetails,
} from '../../types/oauth'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import the service after mocking
import { oauthService } from '../oauthService'

describe('OAuthService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getConsent', () => {
    it('fetches consent details for the (URL-encoded) login id', async () => {
      const details: OAuthConsentDetails = {
        authenticated: true,
        client_name: 'Claude Code',
        redirect_host: 'claude.ai',
        scopes: ['mcp'],
        csrf: 'csrf-1',
      }
      mockApiClient.get.mockResolvedValue(details)

      const result = await oauthService.getConsent('a/b c')

      expect(mockApiClient.get).toHaveBeenCalledWith(
        '/oauth/consent?login=a%2Fb%20c'
      )
      expect(result).toEqual(details)
    })

    it('can return a needs-login (unauthenticated) response', async () => {
      const details: OAuthConsentDetails = { authenticated: false, csrf: 'c' }
      mockApiClient.get.mockResolvedValue(details)

      const result = await oauthService.getConsent('login-1')

      expect(result.authenticated).toBe(false)
    })
  })

  describe('attach', () => {
    it('POSTs the login id with the CSRF token in the X-CSRF-Token header', async () => {
      const response: OAuthConsentAttachResponse = { authenticated: true }
      mockApiClient.post.mockResolvedValue(response)

      const result = await oauthService.attach('login-1', 'csrf-1')

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/oauth/consent/attach',
        { login: 'login-1' },
        { 'X-CSRF-Token': 'csrf-1' }
      )
      expect(result).toEqual(response)
    })

    it('propagates errors (e.g. 401 when signed out)', async () => {
      mockApiClient.post.mockRejectedValue(new Error('unauthorized'))

      await expect(oauthService.attach('login-1', 'csrf-1')).rejects.toThrow(
        'unauthorized'
      )
    })
  })

  describe('submitConsent', () => {
    it('POSTs the decision and returns the redirect target', async () => {
      mockApiClient.post.mockResolvedValue({
        redirect_to: 'https://claude.ai/cb?code=xyz',
      })

      const result = await oauthService.submitConsent(
        'login-1',
        'csrf-1',
        'approve'
      )

      expect(mockApiClient.post).toHaveBeenCalledWith('/oauth/consent', {
        login: 'login-1',
        csrf: 'csrf-1',
        action: 'approve',
      })
      expect(result.redirect_to).toBe('https://claude.ai/cb?code=xyz')
    })
  })
})
