import type { components } from '@vibexp/api-client'

import { apiClient } from '../lib/apiClient'

// Generated wire types for the auth domain — the OpenAPI spec is the single
// source of truth. `LoginUrlResponse` keeps its historical name as an alias of
// the generated `LoginResponse` ({ url }).
export type User = components['schemas']['User']
export type AuthProvider = components['schemas']['AuthProvider']
export type ProvidersResponse = components['schemas']['ProvidersResponse']
export type LoginUrlResponse = components['schemas']['LoginResponse']
export type LogoutResponse = components['schemas']['LogoutResponse']

class AuthService {
  /**
   * List the login providers enabled in this deployment, so the sign-in screen
   * can render a provider picker instead of hardcoding the list. Each provider
   * carries a canonical `name` (passed back to getLoginUrl) and a `display_name`
   * label. The list may be empty when no provider is configured.
   */
  async getProviders(): Promise<AuthProvider[]> {
    const response = await apiClient.get<ProvidersResponse>('/auth/providers')
    return response.providers
  }

  /**
   * Get the identity-provider login URL from the backend.
   * An optional provider name can be passed to select the provider (e.g.
   * 'google', 'github', 'oidc'). When omitted the backend uses its sole enabled
   * provider. The caller should redirect the browser to the returned URL.
   */
  async getLoginUrl(provider?: string): Promise<string> {
    const endpoint = provider
      ? `/auth/login?provider=${encodeURIComponent(provider)}`
      : '/auth/login'
    const response = await apiClient.get<LoginUrlResponse>(endpoint)
    return response.url
  }

  /**
   * Fetch the currently authenticated user via the httpOnly session cookie.
   * Returns the user object if the session is valid, throws on 401/network error.
   */
  async getCurrentUser(): Promise<User> {
    return apiClient.get<User>('/auth/me')
  }

  /**
   * Server-side logout: clears the httpOnly session cookie.
   */
  async logout(): Promise<void> {
    await apiClient.post<LogoutResponse>('/auth/logout')
  }

  /**
   * Mark onboarding as completed for the current user.
   */
  async markOnboardingComplete(): Promise<User> {
    return apiClient.post<User>('/user/onboarding/complete')
  }

  /**
   * Development-only login. Backend sets the session cookie and returns the user.
   * Returns the authenticated User directly (no token).
   */
  async devLogin(email: string, name?: string): Promise<User> {
    return apiClient.post<User>('/auth/dev/login', {
      email,
      name: name ?? 'Dev User',
    })
  }
}

export const authService = new AuthService()
