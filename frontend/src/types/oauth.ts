// Types for the OAuth Authorization Server consent screen (issue #52). The
// consent UI lives in the SPA; these mirror the backend oauthserver JSON shapes
// served under /api/v1/oauth/consent. All OAuth issuance/CSRF/redirect validation
// stays server-side — the page only renders details and echoes the CSRF token.

// OAuthConsentDetails is returned by GET /api/v1/oauth/consent?login=ID.
export interface OAuthConsentDetails {
  /** Human label for the requesting OAuth client (falls back to its client id). */
  client_name: string
  /** Host of the client's redirect URI, shown so the user knows where they go. */
  redirect_host: string
  /** Scopes the client requested. */
  scopes: string[]
  /** CSRF token bound to the login session; echoed back on the decision POST. */
  csrf: string
}

// OAuthConsentAction is the user's decision on the consent screen.
export type OAuthConsentAction = 'approve' | 'deny'

// OAuthConsentDecisionResponse is returned by POST /api/v1/oauth/consent. The SPA
// navigates the browser to redirect_to so the OAuth client receives the code (on
// approve) or error=access_denied (on deny) at its callback.
export interface OAuthConsentDecisionResponse {
  redirect_to: string
}
