/**
 * OAuth provider definitions for WorkOS sign-in.
 *
 * Each entry pairs the WorkOS provider slug (sent to the backend ?provider= param)
 * with the display name used in GA4 method attribution.
 *
 * To add a new provider: add one entry here, then one button in SignInPage.tsx.
 */
export const OAUTH_PROVIDERS = {
  google: {
    slug: 'GoogleOAuth',
    displayName: 'Google',
  },
  github: {
    slug: 'GitHubOAuth',
    displayName: 'GitHub',
  },
  microsoft: {
    slug: 'MicrosoftOAuth',
    displayName: 'Microsoft',
  },
  apple: {
    slug: 'AppleOAuth',
    displayName: 'Apple',
  },
} as const

export type OAuthProviderKey = keyof typeof OAUTH_PROVIDERS
