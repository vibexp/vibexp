import { apiClient } from '../lib/apiClient'
import type {
  OAuthConsentAction,
  OAuthConsentDecisionResponse,
  OAuthConsentDetails,
} from '../types/oauth'

class OAuthService {
  /**
   * Fetch the consent-screen details for an opaque, single-use `login` id (set
   * by the Authorization Server after the user authenticated). Throws on an
   * expired/invalid login session; the page renders a friendly error state.
   */
  async getConsent(login: string): Promise<OAuthConsentDetails> {
    return apiClient.get<OAuthConsentDetails>(
      `/oauth/consent?login=${encodeURIComponent(login)}`
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
