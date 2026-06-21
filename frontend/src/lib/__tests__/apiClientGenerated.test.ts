import { ApiError } from '../../types/errors'
import { unwrap } from '../apiClientGenerated'

const response = (status: number, statusText = ''): Response =>
  ({ ok: status >= 200 && status < 300, status, statusText }) as Response

const problemBody = {
  type: 'https://api.vibexp.io/errors/RESOURCE_NOT_FOUND',
  title: 'Not Found',
  status: 404,
  detail: 'Notification not found',
  code: 'RESOURCE_NOT_FOUND',
  request_id: 'req-123',
  timestamp: '2026-06-10T10:00:00Z',
}

describe('unwrap', () => {
  it('resolves with data on success', async () => {
    const data = { unread_count: 3 }

    await expect(
      unwrap(Promise.resolve({ data, response: response(200, 'OK') }))
    ).resolves.toEqual({ unread_count: 3 })
  })

  it('resolves with undefined for 204 No Content', async () => {
    await expect(
      unwrap(
        Promise.resolve({
          data: undefined,
          response: response(204, 'No Content'),
        })
      )
    ).resolves.toBeUndefined()
  })

  it('throws ApiError preserving RFC 9457 fields', async () => {
    const promise = unwrap(
      Promise.resolve({
        error: problemBody,
        response: response(404, 'Not Found'),
      })
    )

    await expect(promise).rejects.toThrow(ApiError)
    await expect(promise).rejects.toMatchObject({
      status: 404,
      code: 'RESOURCE_NOT_FOUND',
      requestId: 'req-123',
      message: 'Notification not found',
    })
  })

  it('throws generic ApiError when the error body is not problem details', async () => {
    const promise = unwrap(
      Promise.resolve({
        error: { message: 'something broke' },
        response: response(500, 'Internal Server Error'),
      })
    )

    await expect(promise).rejects.toThrow(ApiError)
    await expect(promise).rejects.toMatchObject({
      status: 500,
      code: 'UNKNOWN_ERROR',
      message: 'HTTP 500 error',
    })
  })

  it('uses a plain-text error body as the detail', async () => {
    await expect(
      unwrap(
        Promise.resolve({
          error: 'service unavailable',
          response: response(503, 'Service Unavailable'),
        })
      )
    ).rejects.toMatchObject({
      status: 503,
      code: 'UNKNOWN_ERROR',
      message: 'service unavailable',
    })
  })

  it('throws generic ApiError for a body-less failure (error: undefined)', async () => {
    await expect(
      unwrap(
        Promise.resolve({
          error: undefined,
          response: response(401, 'Unauthorized'),
        })
      )
    ).rejects.toMatchObject({
      status: 401,
      code: 'UNKNOWN_ERROR',
      message: 'HTTP 401 error',
    })
  })

  it('wraps fetch TypeError rejections as a network error', async () => {
    await expect(
      unwrap(Promise.reject(new TypeError('Failed to fetch')))
    ).rejects.toThrow('Network error: Unable to connect to server')
  })

  it('wraps AbortSignal.timeout rejections as a request timeout', async () => {
    await expect(
      unwrap(
        Promise.reject(new DOMException('signal timed out', 'TimeoutError'))
      )
    ).rejects.toThrow('Request timeout: the server took too long to respond')
  })

  it('re-throws non-TypeError rejections unchanged', async () => {
    const abort = new DOMException('Aborted', 'AbortError')

    await expect(unwrap(Promise.reject(abort))).rejects.toBe(abort)
  })
})
