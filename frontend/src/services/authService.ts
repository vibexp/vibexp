import { apiClient } from '../lib/apiClient'
import type { LoginUrlResponse, LogoutResponse, User } from '../types'

class AuthService {
  /**
   * Get the WorkOS login URL from the backend.
   * An optional provider slug can be passed to pre-select the OAuth provider
   * (e.g. 'GoogleOAuth', 'GitHubOAuth'). When omitted the backend uses its default.
   * The caller should redirect the browser to the returned URL.
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
