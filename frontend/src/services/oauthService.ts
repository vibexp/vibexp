import { apiClient } from '../lib/apiClient'
import type {
  OAuthConsentAction,
  OAuthConsentAttachResponse,
  OAuthConsentDecisionResponse,
  OAuthConsentDetails,
} from '../types/oauth'

class OAuthService {
  /**
   * Fetch the consent-screen details for an opaque, single-use `login` id (set
   * by the Authorization Server). Returns `{ authenticated: false }` until an
   * app user is bound to the session (via {@link attach}); once bound it
   * carries the approval-screen fields. Throws on an expired/invalid login
   * session; the page renders a friendly error state.
   */
  async getConsent(login: string): Promise<OAuthConsentDetails> {
    return apiClient.get<OAuthConsentDetails>(
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
    return apiClient.post<OAuthConsentAttachResponse>(
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
    return apiClient.post<OAuthConsentDecisionResponse>('/oauth/consent', {
      login,
      csrf,
      action,
    })
  }
}

export const oauthService = new OAuthService()
