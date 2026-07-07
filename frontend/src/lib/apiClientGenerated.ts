import { createApiClient } from '@vibexp/api-client'

import { errorTypeUri } from '../config/siteConfig'
import { ApiError, type APIErrorResponse } from '../types/errors'
import { getApiBaseUrl } from '../utils/environment'

// Generated-spec paths already include the `/api/v1/...` prefix, so the
// client needs the bare API origin rather than the hand-written client's
// versioned base URL.
const API_ORIGIN = getApiBaseUrl().replace(/\/api\/v1\/?$/, '')

// Same default as the hand-written client — without it a stalled request
// would hold the hooks' in-flight guards forever.
const REQUEST_TIMEOUT_MS = 30000

/**
 * Typed openapi-fetch client generated from the backend OpenAPI spec.
 *
 * Side-by-side with the hand-written `apiClient`: domains migrate to this
 * client incrementally (see docs/developer-guidelines/frontend/api-integration.md).
 * Authentication uses the same httpOnly session cookie as `apiClient`.
 */
export const generatedClient = createApiClient({
  baseUrl: API_ORIGIN,
  credentials: 'include',
  // Combine the caller's signal (openapi-fetch puts a per-request `signal` on
  // the Request) with the request timeout so callers can still cancel in-flight
  // requests (e.g. charts aborting on unmount / range change) while the timeout
  // guard is preserved. `AbortSignal.any` aborts as soon as either fires.
  fetch: request =>
    fetch(request, {
      signal: AbortSignal.any([
        request.signal,
        AbortSignal.timeout(REQUEST_TIMEOUT_MS),
      ]),
    }),
})

interface FetchResult<T> {
  data?: T
  error?: unknown
  response: Response
}

function isProblemDetails(body: unknown): body is APIErrorResponse {
  if (typeof body !== 'object' || body === null) return false
  const candidate = body as Partial<APIErrorResponse>
  return (
    typeof candidate.code === 'string' &&
    candidate.code !== '' &&
    typeof candidate.detail === 'string' &&
    candidate.detail !== ''
  )
}

function toApiError(error: unknown, response: Response): ApiError {
  if (isProblemDetails(error)) {
    return new ApiError(error)
  }

  const genericDetail =
    typeof error === 'string' && error !== ''
      ? error
      : `HTTP ${String(response.status)} error`

  return new ApiError({
    type: errorTypeUri('UNKNOWN'),
    title: response.statusText !== '' ? response.statusText : 'Error',
    status: response.status,
    detail: genericDetail,
    code: 'UNKNOWN_ERROR',
    request_id: '',
    timestamp: new Date().toISOString(),
  })
}

/**
 * Resolve an openapi-fetch call to its typed payload, throwing the same
 * `ApiError` the hand-written `apiClient` throws so callers (toasts, hooks)
 * behave identically regardless of which client served the request.
 *
 * Error responses go off `response.ok`, not the presence of `error` —
 * openapi-fetch yields `{ error: undefined }` for body-less failures (e.g.
 * an errored 204/Content-Length:0 response).
 */
export async function unwrap<T>(request: Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>
  try {
    result = await request
  } catch (error) {
    if (error instanceof DOMException && error.name === 'TimeoutError') {
      throw new Error('Request timeout: the server took too long to respond')
    }
    if (error instanceof TypeError) {
      throw new Error('Network error: Unable to connect to server')
    }
    throw error
  }

  if (!result.response.ok) {
    throw toApiError(result.error, result.response)
  }

  // 204 No Content resolves with `data: undefined`; callers of body-less
  // operations type the result as void.
  return result.data as T
}
