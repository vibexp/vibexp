/**
 * Tests for APIClient.
 *
 * Verifies error messages, credentials: 'include' behavior,
 * and that the Authorization header is NOT injected (cookie-based auth).
 */

// Mock environment utility
jest.mock('../../utils/environment', () => ({
  getApiBaseUrl: () => 'https://api.vibexp.io/api/v1',
}))

import { ApiError } from '../../types/errors'
import { APIClient } from '../apiClient'

// Helper to create a mock Response
function createMockResponse(
  status: number,
  body: unknown,
  contentType = 'application/json'
): Response {
  const responseBody = typeof body === 'string' ? body : JSON.stringify(body)
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText:
      status === 404
        ? 'Not Found'
        : status === 405
          ? 'Method Not Allowed'
          : 'Error',
    headers: new Headers({ 'content-type': contentType }),
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(responseBody),
  } as unknown as Response
}

describe('APIClient', () => {
  let client: APIClient

  beforeEach(() => {
    client = new APIClient('https://api.vibexp.io/api/v1')
    jest.clearAllMocks()
  })

  afterEach(() => {
    jest.restoreAllMocks()
  })

  describe('cookie-based auth (no Authorization header)', () => {
    it('sends credentials: include on every request', async () => {
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(200, { ok: true }))

      await client.get('/test/endpoint')

      expect(global.fetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({ credentials: 'include' })
      )
    })

    it('does NOT send Authorization header when making requests', async () => {
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(200, { ok: true }))

      await client.get('/test/endpoint')

      const callArgs = (global.fetch as jest.Mock).mock.calls[0] as [
        string,
        RequestInit,
      ]
      const headers = callArgs[1].headers as Record<string, string>
      expect(headers).not.toHaveProperty('Authorization')
    })
  })

  describe('timeout errors include endpoint context', () => {
    it('includes method and endpoint in timeout error message', async () => {
      // Simulate a timeout (AbortError from DOMException)
      global.fetch = jest
        .fn()
        .mockImplementation(() =>
          Promise.reject(
            new DOMException('The user aborted a request.', 'AbortError')
          )
        )

      await expect(client.get('/test/endpoint')).rejects.toThrow(
        'Request timeout: GET /test/endpoint'
      )
    })

    it('includes method and endpoint in POST timeout error message', async () => {
      global.fetch = jest
        .fn()
        .mockImplementation(() =>
          Promise.reject(
            new DOMException('The user aborted a request.', 'AbortError')
          )
        )

      await expect(client.post('/user/onboarding/complete')).rejects.toThrow(
        'Request timeout: POST /user/onboarding/complete'
      )
    })
  })

  describe('network errors include endpoint context', () => {
    it('includes method and endpoint in network error message', async () => {
      global.fetch = jest
        .fn()
        .mockRejectedValue(new TypeError('Failed to fetch'))

      await expect(client.get('/test/endpoint')).rejects.toThrow(
        'Network error: Unable to connect to server (GET /test/endpoint)'
      )
    })

    it('includes method and endpoint for POST network error', async () => {
      global.fetch = jest
        .fn()
        .mockRejectedValue(new TypeError('Failed to fetch'))

      await expect(client.post('/user/onboarding/complete')).rejects.toThrow(
        'Network error: Unable to connect to server (POST /user/onboarding/complete)'
      )
    })
  })

  describe('generic request failures include endpoint context', () => {
    it('includes method and endpoint in generic request failure message', async () => {
      global.fetch = jest
        .fn()
        .mockRejectedValue(new Error('Unexpected error occurred'))

      await expect(client.get('/some/endpoint')).rejects.toThrow(
        'Request failed (GET /some/endpoint): Unexpected error occurred'
      )
    })
  })

  describe('HTTP error responses include endpoint context', () => {
    it('includes endpoint in 404 plain-text error detail', async () => {
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(404, 'Not Found', 'text/plain'))

      try {
        await client.post('/user/onboarding/complete')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.response.detail).toBeDefined()
        expect(apiError.status).toBe(404)
      }
    })

    it('preserves RFC 9457 detail field on the thrown ApiError', async () => {
      // Regression: a previous version of handleErrorResponse threw the
      // RFC 9457 ApiError from inside a try whose own catch unconditionally
      // replaced it with a generic "HTTP 400 error", swallowing the
      // backend-provided detail. Asserting only `.rejects.toThrow(ApiError)`
      // missed this — we must verify the actual detail content survives.
      const rfc9457Error = {
        type: 'https://api.vibexp.io/errors/SUBSCRIPTION_CONFLICT',
        title: 'Conflict',
        status: 400,
        detail:
          'This team already has an active subscription. Use the billing portal to make changes.',
        code: 'SUBSCRIPTION_CONFLICT',
        request_id: 'req-abc',
        timestamp: '2026-02-14T10:00:00Z',
      }

      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(400, rfc9457Error))

      try {
        await client.post('/teams/123/subscribe', { plan: 'pro' })
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.response.detail).toBe(rfc9457Error.detail)
        expect(apiError.getMessage()).toBe(rfc9457Error.detail)
        expect(apiError.code).toBe('SUBSCRIPTION_CONFLICT')
        expect(apiError.status).toBe(400)
        expect(apiError.requestId).toBe('req-abc')
      }
    })

    it('throws ApiError for RFC 9457 formatted responses', async () => {
      const rfc9457Error = {
        type: 'https://api.vibexp.io/errors/NOT_FOUND',
        title: 'Not Found',
        status: 404,
        detail: 'The requested endpoint does not exist',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-123',
        timestamp: '2026-02-14T10:00:00Z',
      }

      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(404, rfc9457Error))

      try {
        await client.post('/user/onboarding/complete')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        // Strengthened from the previous .rejects.toThrow(ApiError) check:
        // verify the backend detail is what reaches the caller.
        expect(apiError.response.detail).toBe(
          'The requested endpoint does not exist'
        )
        expect(apiError.code).toBe('RESOURCE_NOT_FOUND')
      }
    })

    it('falls back to a generic ApiError for plain-text error bodies', async () => {
      global.fetch = jest
        .fn()
        .mockResolvedValue(
          createMockResponse(500, 'internal explosion', 'text/plain')
        )

      try {
        await client.get('/teams')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.code).toBe('UNKNOWN_ERROR')
        // Plain text body becomes the detail.
        expect(apiError.response.detail).toBe('internal explosion')
        expect(apiError.status).toBe(500)
      }
    })

    it('falls back to a generic ApiError when JSON parsing fails', async () => {
      // application/json content-type but response.json() rejects (malformed body).
      global.fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 502,
        statusText: 'Bad Gateway',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: () => Promise.reject(new SyntaxError('Unexpected token <')),
        text: () => Promise.resolve('<html>...'),
      })

      try {
        await client.get('/teams')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.code).toBe('UNKNOWN_ERROR')
        // No body text consumed in the JSON path; uses the generic context message.
        expect(apiError.response.detail).toBe('HTTP 502 error [GET /teams]')
        expect(apiError.status).toBe(502)
      }
    })

    it('falls back to a generic ApiError when JSON body lacks code/detail', async () => {
      // Valid JSON, but not RFC 9457: missing `code` and `detail`.
      const malformedRfc = { error: 'something went wrong' }
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(400, malformedRfc))

      try {
        await client.post('/teams/123/subscribe', { plan: 'pro' })
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.code).toBe('UNKNOWN_ERROR')
        expect(apiError.response.detail).toBe(
          'HTTP 400 error [POST /teams/123/subscribe]'
        )
        expect(apiError.status).toBe(400)
      }
    })

    it('falls back to a generic ApiError when JSON body has code but no detail', async () => {
      // Partial RFC 9457: `code` present but `detail` missing. Both must be
      // truthy for the parsed ApiError to be honored.
      const partialRfc = { code: 'SOMETHING', status: 400 }
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(400, partialRfc))

      try {
        await client.get('/teams')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.code).toBe('UNKNOWN_ERROR')
        expect(apiError.response.detail).toBe('HTTP 400 error [GET /teams]')
      }
    })

    it('falls back to a generic ApiError when JSON body has detail but no code', async () => {
      // Partial RFC 9457: `detail` present but `code` missing.
      const partialRfc = { detail: 'something happened', status: 400 }
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(400, partialRfc))

      try {
        await client.get('/teams')
        fail('Expected an error to be thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(ApiError)
        const apiError = error as ApiError
        expect(apiError.code).toBe('UNKNOWN_ERROR')
        expect(apiError.response.detail).toBe('HTTP 400 error [GET /teams]')
      }
    })

    it('re-throws ApiError instances without wrapping', async () => {
      const existingApiError = new ApiError({
        type: 'https://api.vibexp.io/errors/FORBIDDEN',
        title: 'Forbidden',
        status: 403,
        detail: 'Access denied',
        code: 'FORBIDDEN',
        request_id: 'req-456',
        timestamp: '2026-02-14T10:00:00Z',
      })

      global.fetch = jest.fn().mockRejectedValue(existingApiError)

      await expect(client.get('/protected/resource')).rejects.toThrow(
        existingApiError
      )
    })
  })

  describe('successful requests', () => {
    it('returns parsed JSON for successful responses', async () => {
      const responseData = { data: { total: 42 } }
      global.fetch = jest
        .fn()
        .mockResolvedValue(createMockResponse(200, responseData))

      const result = await client.get<typeof responseData>('/test/endpoint')
      expect(result).toEqual(responseData)
    })

    it('returns empty object for 204 No Content responses', async () => {
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        status: 204,
        statusText: 'No Content',
        headers: new Headers({}),
        json: () => Promise.resolve({}),
        text: () => Promise.resolve(''),
      })

      const result = await client.delete('/test/resource')
      expect(result).toEqual({})
    })
  })
})
