import { errorTypeUri } from '../config/siteConfig'
import { ApiError, type APIErrorResponse } from '../types/errors'
import { getApiBaseUrl } from '../utils/environment'

const API_BASE_URL = getApiBaseUrl()
const DEFAULT_TIMEOUT = 30000 // 30 seconds

interface RequestConfig {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  endpoint: string
  data?: unknown
  headers?: Record<string, string>
  timeout?: number
  signal?: AbortSignal
}

/**
 * Centralized API client with automatic error parsing and handling
 */
class APIClient {
  private baseURL: string
  private defaultTimeout: number

  constructor(
    baseURL: string = API_BASE_URL,
    timeout: number = DEFAULT_TIMEOUT
  ) {
    this.baseURL = baseURL
    this.defaultTimeout = timeout
  }

  /**
   * Make HTTP request with automatic error handling.
   * Authentication is handled via httpOnly session cookie (credentials: 'include').
   */
  private async request<T>(config: RequestConfig): Promise<T> {
    const {
      method,
      endpoint,
      data,
      headers = {},
      timeout = this.defaultTimeout,
      signal: externalSignal,
    } = config
    const url = `${this.baseURL}${endpoint}`

    // FormData (file uploads) must NOT be JSON-encoded, and the browser must set
    // its own multipart Content-Type with the boundary — so skip both for it.
    const isFormData =
      typeof FormData !== 'undefined' && data instanceof FormData

    // Add content type for POST/PUT/PATCH JSON requests
    if (data && !isFormData && !headers['Content-Type']) {
      headers['Content-Type'] = 'application/json'
    }

    // Create abort controller for timeout; combine with external signal if provided
    const controller = new AbortController()
    const timeoutId = setTimeout(() => {
      controller.abort()
    }, timeout)

    // If an external signal is provided, abort our controller when it fires
    if (externalSignal) {
      if (externalSignal.aborted) {
        clearTimeout(timeoutId)
        throw new DOMException('Aborted', 'AbortError')
      }
      externalSignal.addEventListener(
        'abort',
        () => {
          controller.abort()
        },
        { once: true }
      )
    }

    try {
      const response = await fetch(url, {
        method,
        headers,
        body: data
          ? data instanceof FormData
            ? data
            : JSON.stringify(data)
          : undefined,
        signal: controller.signal,
        credentials: 'include',
      })

      clearTimeout(timeoutId)

      // Handle error responses
      if (!response.ok) {
        await this.handleErrorResponse(response, method, endpoint)
      }

      // Parse successful response
      const contentType = response.headers.get('content-type')
      if (contentType?.includes('application/json')) {
        // External API response - we trust the API to return the expected type
        return (await response.json()) as T
      }

      // For 204 No Content or non-JSON responses
      return {} as T
    } catch (error) {
      clearTimeout(timeoutId)

      // Handle timeout
      if (error instanceof DOMException && error.name === 'AbortError') {
        throw new Error(`Request timeout: ${method} ${endpoint}`)
      }

      // Handle network errors
      if (error instanceof TypeError) {
        throw new Error(
          `Network error: Unable to connect to server (${method} ${endpoint})`
        )
      }

      // Re-throw ApiError instances
      if (error instanceof ApiError) {
        throw error
      }

      // Wrap other errors
      throw new Error(
        `Request failed (${method} ${endpoint}): ${error instanceof Error ? error.message : 'Unknown error'}`
      )
    }
  }

  /**
   * Handle error response and throw ApiError.
   *
   * Preserves the backend's RFC 9457 `detail` when present so callers (and
   * the toast layer) can show actionable messages. Falls back to a generic
   * `HTTP <status> error` only when the body is non-JSON, malformed, or
   * missing the required `code`/`detail` fields.
   */
  private async handleErrorResponse(
    response: Response,
    method?: string,
    endpoint?: string
  ): Promise<never> {
    const context = method && endpoint ? ` [${method} ${endpoint}]` : ''
    const contentType = response.headers.get('content-type')
    const isJsonLike =
      contentType !== null &&
      (contentType.includes('application/problem+json') ||
        contentType.includes('application/json'))

    let bodyText = ''
    if (isJsonLike) {
      try {
        // External API error response - we trust the API to return RFC 9457 format
        const parsed = (await response.json()) as APIErrorResponse
        if (parsed.code && parsed.detail) {
          throw new ApiError(parsed)
        }
      } catch (e) {
        // Re-throw the ApiError we just constructed; let everything else
        // (json() rejecting, parsed body missing fields) fall through to
        // the generic fallback below.
        if (e instanceof ApiError) throw e
      }
    } else {
      try {
        bodyText = await response.text()
      } catch {
        // Body unreadable; fall through to generic fallback.
      }
    }

    const genericDetail =
      bodyText !== ''
        ? bodyText
        : `HTTP ${String(response.status)} error${context}`
    const title = response.statusText !== '' ? response.statusText : 'Error'

    throw new ApiError({
      type: errorTypeUri('UNKNOWN'),
      title,
      status: response.status,
      detail: genericDetail,
      code: 'UNKNOWN_ERROR',
      request_id: '',
      timestamp: new Date().toISOString(),
    })
  }

  /**
   * GET request
   */
  async get<T>(
    endpoint: string,
    options?: { headers?: Record<string, string>; signal?: AbortSignal }
  ): Promise<T> {
    return this.request<T>({
      method: 'GET',
      endpoint,
      headers: options?.headers,
      signal: options?.signal,
    })
  }

  /**
   * POST request
   */
  async post<T>(
    endpoint: string,
    data?: unknown,
    headers?: Record<string, string>
  ): Promise<T> {
    return this.request<T>({ method: 'POST', endpoint, data, headers })
  }

  /**
   * PUT request
   */
  async put<T>(
    endpoint: string,
    data?: unknown,
    headers?: Record<string, string>
  ): Promise<T> {
    return this.request<T>({ method: 'PUT', endpoint, data, headers })
  }

  /**
   * PATCH request
   */
  async patch<T>(
    endpoint: string,
    data?: unknown,
    headers?: Record<string, string>
  ): Promise<T> {
    return this.request<T>({ method: 'PATCH', endpoint, data, headers })
  }

  /**
   * DELETE request
   * @param data - Optional request body (use when the endpoint requires a body, e.g. bulk deletes)
   */
  async delete<T>(
    endpoint: string,
    data?: unknown,
    headers?: Record<string, string>
  ): Promise<T> {
    return this.request<T>({ method: 'DELETE', endpoint, data, headers })
  }
}

// Export singleton instance
export const apiClient = new APIClient()

// Export class for testing
export { APIClient }
