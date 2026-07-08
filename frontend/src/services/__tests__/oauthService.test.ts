// The OAuth consent surface is intentionally hand-written (endpoints live on the
// embedded AS, outside openapi.yaml), so oauthService uses a local fetch wrapper
// rather than the generated client. Mock global fetch and assert on the request
// path (the API base-URL prefix is environment-driven and asserted loosely).
import { ApiError } from '../../types/errors'
import type {
  OAuthConsentAttachResponse,
  OAuthConsentDetails,
} from '../../types/oauth'
import { oauthService } from '../oauthService'

interface ResponseInit {
  ok?: boolean
  status?: number
  statusText?: string
  contentType?: string | null
}

function makeResponse(body: unknown, init: ResponseInit = {}): Response {
  const ok = init.ok ?? true
  const contentType =
    init.contentType === undefined ? 'application/json' : init.contentType
  return {
    ok,
    status: init.status ?? (ok ? 200 : 400),
    statusText: init.statusText ?? (ok ? 'OK' : 'Error'),
    headers: {
      get: (name: string) =>
        name.toLowerCase() === 'content-type' ? contentType : null,
    },
    json: () => Promise.resolve(body),
  } as unknown as Response
}

describe('OAuthService', () => {
  let fetchMock: jest.Mock

  beforeEach(() => {
    jest.clearAllMocks()
    fetchMock = jest.fn()
    global.fetch = fetchMock
  })

  describe('getConsent', () => {
    it('GETs consent details for the (URL-encoded) login id with session cookie', async () => {
      const details: OAuthConsentDetails = {
        authenticated: true,
        client_name: 'Claude Code',
        redirect_host: 'claude.ai',
        scopes: ['mcp'],
        csrf: 'csrf-1',
      }
      fetchMock.mockResolvedValue(makeResponse(details))

      const result = await oauthService.getConsent('a/b c')

      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/oauth/consent?login=a%2Fb%20c'),
        {
          method: 'GET',
          headers: {},
          body: undefined,
          credentials: 'include',
        }
      )
      expect(result).toEqual(details)
    })

    it('can return a needs-login (unauthenticated) response', async () => {
      const details: OAuthConsentDetails = { authenticated: false, csrf: 'c' }
      fetchMock.mockResolvedValue(makeResponse(details))

      const result = await oauthService.getConsent('login-1')

      expect(result.authenticated).toBe(false)
    })
  })

  describe('attach', () => {
    it('POSTs the login id with the CSRF token in the X-CSRF-Token header', async () => {
      const response: OAuthConsentAttachResponse = { authenticated: true }
      fetchMock.mockResolvedValue(makeResponse(response))

      const result = await oauthService.attach('login-1', 'csrf-1')

      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/oauth/consent/attach'),
        {
          method: 'POST',
          headers: {
            'X-CSRF-Token': 'csrf-1',
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ login: 'login-1' }),
          credentials: 'include',
        }
      )
      expect(result).toEqual(response)
    })

    it('maps an RFC 9457 error body to ApiError, preserving status (e.g. 401 when signed out)', async () => {
      fetchMock.mockResolvedValue(
        makeResponse(
          {
            type: 'about:blank',
            title: 'Unauthorized',
            status: 401,
            detail: 'unauthorized',
            code: 'AUTH_REQUIRED',
            request_id: 'req-1',
            timestamp: '2026-07-08T00:00:00Z',
          },
          { ok: false, status: 401, contentType: 'application/problem+json' }
        )
      )

      const error = await oauthService
        .attach('login-1', 'csrf-1')
        .catch((e: unknown) => e)

      expect(error).toBeInstanceOf(ApiError)
      expect((error as ApiError).status).toBe(401)
      expect((error as ApiError).message).toBe('unauthorized')
    })
  })

  describe('submitConsent', () => {
    it('POSTs the decision and returns the redirect target', async () => {
      fetchMock.mockResolvedValue(
        makeResponse({ redirect_to: 'https://claude.ai/cb?code=xyz' })
      )

      const result = await oauthService.submitConsent(
        'login-1',
        'csrf-1',
        'approve'
      )

      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/oauth/consent'),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            login: 'login-1',
            csrf: 'csrf-1',
            action: 'approve',
          }),
          credentials: 'include',
        }
      )
      expect(result.redirect_to).toBe('https://claude.ai/cb?code=xyz')
    })

    it('throws a generic ApiError when the error body is not problem details', async () => {
      fetchMock.mockResolvedValue(
        makeResponse('nope', {
          ok: false,
          status: 500,
          statusText: 'Internal Server Error',
          contentType: 'text/plain',
        })
      )

      const error = await oauthService
        .submitConsent('login-1', 'csrf-1', 'approve')
        .catch((e: unknown) => e)

      expect(error).toBeInstanceOf(ApiError)
      expect((error as ApiError).status).toBe(500)
      expect((error as ApiError).code).toBe('UNKNOWN_ERROR')
    })
  })
})
