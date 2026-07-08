import { errorTypeUri } from '../config/siteConfig'
import { ApiError, type APIErrorResponse } from '../types/errors'
import type {
  OAuthConsentAction,
  OAuthConsentAttachResponse,
  OAuthConsentDecisionResponse,
  OAuthConsentDetails,
} from '../types/oauth'
import { getApiBaseUrl } from '../utils/environment'

const API_BASE_URL = getApiBaseUrl()

/**
 * Minimal, self-contained fetch wrapper for the OAuth Authorization Server
 * consent surface (`/oauth/consent[/attach]`).
 *
 * INTENTIONALLY HAND-WRITTEN — the deliberate exception to the "every service
 * uses `generatedClient`" rule (see docs/developer-guidelines/frontend/api-integration.md).
 * These endpoints are served by the embedded AS and are kept OUT of
 * `openapi.yaml` by design: documenting them would break the spec drift +
 * payload-coverage gates (the drift test's DB-free server never mounts the AS
 * routes) — re-verified in #89 (PR #100), same conclusion as #34/PR #72. Because
 * they are not in the spec, they are not in the generated client, so this thin
 * wrapper reproduces just what the consent flow needs: same-origin session
 * cookie auth, JSON encoding, and the RFC 9457 → {@link ApiError} mapping the
 * rest of the app relies on (the consent page branches on `ApiError.status`).
 * Keep it small and local; do not grow it into a second general-purpose client.
 */
async function consentRequest<T>(
  method: 'GET' | 'POST',
  endpoint: string,
  body?: unknown,
  headers: Record<string, string> = {}
): Promise<T> {
  const requestHeaders: Record<string, string> = { ...headers }
  if (body !== undefined) {
    requestHeaders['Content-Type'] = 'application/json'
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method,
    headers: requestHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'include',
  })

  if (!response.ok) {
    await throwConsentError(response)
  }

  const contentType = response.headers.get('content-type')
  if (contentType?.includes('application/json')) {
    return (await response.json()) as T
  }

  // 204 / non-JSON responses have no body to parse.
  return {} as T
}

/**
 * Parse an error response into the same {@link ApiError} the generated client's
 * `unwrap` throws, preserving the backend's RFC 9457 `detail`/`code` when
 * present so callers behave identically regardless of transport.
 */
async function throwConsentError(response: Response): Promise<never> {
  const contentType = response.headers.get('content-type')
  const isJsonLike =
    contentType !== null &&
    (contentType.includes('application/problem+json') ||
      contentType.includes('application/json'))

  if (isJsonLike) {
    try {
      const parsed = (await response.json()) as APIErrorResponse
      if (parsed.code && parsed.detail) {
        throw new ApiError(parsed)
      }
    } catch (e) {
      // Re-throw the ApiError we just built; a json() rejection or a body
      // missing code/detail falls through to the generic error below.
      if (e instanceof ApiError) throw e
    }
  }

  throw new ApiError({
    type: errorTypeUri('UNKNOWN'),
    title: response.statusText !== '' ? response.statusText : 'Error',
    status: response.status,
    detail: `HTTP ${String(response.status)} error`,
    code: 'UNKNOWN_ERROR',
    request_id: '',
    timestamp: new Date().toISOString(),
  })
}

class OAuthService {
  /**
   * Fetch the consent-screen details for an opaque, single-use `login` id (set
   * by the Authorization Server). Returns `{ authenticated: false }` until an
   * app user is bound to the session (via {@link attach}); once bound it
   * carries the approval-screen fields. Throws on an expired/invalid login
   * session; the page renders a friendly error state.
   */
  async getConsent(login: string): Promise<OAuthConsentDetails> {
    return consentRequest<OAuthConsentDetails>(
      'GET',
      `/oauth/consent?login=${encodeURIComponent(login)}`
    )
  }

  /**
   * Bind the authenticated app user (resolved from the vx_session cookie) to
   * the AS login session, so consent can proceed. The CSRF token from
   * getConsent is sent as the X-CSRF-Token header. Throws ApiError 401 when no
   * app session exists (caller should redirect to login), 400 on a bad token /
   * expired session, or 409 if the session is already bound to another user.
   */
  async attach(
    login: string,
    csrf: string
  ): Promise<OAuthConsentAttachResponse> {
    return consentRequest<OAuthConsentAttachResponse>(
      'POST',
      '/oauth/consent/attach',
      { login },
      { 'X-CSRF-Token': csrf }
    )
  }

  /**
   * Submit the user's approve/deny decision. Returns the URL the browser must
   * navigate to so the OAuth client receives the authorization code (approve) or
   * error=access_denied (deny) at its callback. The CSRF token comes from
   * getConsent and is verified server-side.
   */
  async submitConsent(
    login: string,
    csrf: string,
    action: OAuthConsentAction
  ): Promise<OAuthConsentDecisionResponse> {
    return consentRequest<OAuthConsentDecisionResponse>(
      'POST',
      '/oauth/consent',
      { login, csrf, action }
    )
  }
}

export const oauthService = new OAuthService()
