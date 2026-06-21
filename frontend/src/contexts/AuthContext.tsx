import {
  createContext,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from 'react'

import { STORAGE_KEYS } from '../constants/storageKeys'
import { analyticsService } from '../services/analytics'
import { authService } from '../services/authService'
import type { User } from '../types'
import { grantCookieConsent } from '../utils/cookieConsent'
import { sessionStore } from '../utils/storage'
import { isFirstTimeUser } from '../utils/userUtils'

export interface AuthContextType {
  user: User | null
  isAuthenticated: boolean
  login: (providerSlug?: string) => Promise<void>
  logout: () => void
  isLoading: boolean
  checkPendingInvitation: () => string | null
  markOnboardingComplete: () => Promise<void>
}

export const AuthContext = createContext<AuthContextType | undefined>(undefined)

interface AuthProviderProps {
  children: ReactNode
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<User | null>(null)
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  // analyticsFiredRef prevents duplicate sign_up / login / purchase events
  // when React 18 StrictMode invokes effects twice in development. We still
  // want /auth/me to run on every mount (it sets the user state), but the
  // analytics push must be idempotent for the lifetime of this provider.
  const analyticsFiredRef = useRef(false)

  // Hydrate session from httpOnly cookie on mount by calling GET /auth/me
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const currentUser = await authService.getCurrentUser()
        setUser(currentUser)
        setIsAuthenticated(true)

        // Read and clear the provider written by SignInPage before the OAuth redirect.
        // Must happen BEFORE the analyticsFiredRef guard so it always clears even
        // when the analytics block is skipped (e.g. React 18 StrictMode double-fire).
        const loginMethod =
          sessionStore.get(STORAGE_KEYS.LOGIN_METHOD) ?? 'unknown'
        sessionStore.remove(STORAGE_KEYS.LOGIN_METHOD)

        if (analyticsFiredRef.current) {
          setIsLoading(false)
          return
        }
        analyticsFiredRef.current = true

        // Auto-grant cookie consent since user agreed to privacy policy during signup
        try {
          grantCookieConsent()
        } catch (consentError) {
          console.error('Failed to grant cookie consent:', consentError)
        }

        // Set GA4 user_id for cross-session tracking
        // This associates all subsequent GA4 events with the logged-in user
        try {
          window.gtag('set', 'user_id', currentUser.id)
        } catch (ga4UserIdError) {
          console.error('Failed to set GA4 user_id:', ga4UserIdError)
        }

        // Track successful authentication and identify user for analytics
        try {
          analyticsService.identify({
            user_id: currentUser.id,
            email: currentUser.email,
            name: currentUser.name,
            signup_date: currentUser.created_at,
            avatar_url: currentUser.avatar_url ?? null,
            created_at: currentUser.created_at,
          })

          // Determine if this is a first-time sign-in based on account creation time
          // Uses the shared utility function (same logic as Homepage Welcome message)
          const isFirstTime = isFirstTimeUser(currentUser.created_at)

          analyticsService.trackAuth({
            eventType: isFirstTime ? 'signed_in_first_time' : 'signed_in',
            userProperties: {
              user_id: currentUser.id,
              email: currentUser.email,
              name: currentUser.name,
              signup_date: currentUser.created_at,
              avatar_url: currentUser.avatar_url ?? null,
              created_at: currentUser.created_at,
            },
          })

          // Track GA4 ecommerce events
          // Wrapped in try-catch to ensure GA4 tracking never affects authentication
          try {
            if (isFirstTime) {
              // Track sign_up
              window.dataLayer.push({
                event: 'sign_up',
                method: loginMethod,
                user_id: currentUser.id,
              })
            } else {
              // Track login
              window.dataLayer.push({
                event: 'login',
                method: loginMethod,
                user_id: currentUser.id,
              })
            }
          } catch (ga4Error) {
            // Log GA4 tracking errors separately, but never let them affect authentication
            console.error('Failed to track GA4 ecommerce events:', ga4Error)
          }
        } catch (analyticsError) {
          console.error('Failed to track authentication event:', analyticsError)
        }
      } catch {
        // 401 means unauthenticated — silently clear state, no error logged
        setUser(null)
        setIsAuthenticated(false)
      } finally {
        setIsLoading(false)
      }
    }

    void checkAuth()
  }, [])

  const login = async (providerSlug?: string) => {
    setIsLoading(true)
    try {
      const loginUrl = await authService.getLoginUrl(providerSlug)
      // Redirect to WorkOS OAuth flow
      window.location.href = loginUrl
    } catch (error) {
      setIsLoading(false)
      throw error
    }
  }

  const logout = () => {
    // Clear GA4 user_id before logout
    try {
      window.gtag('set', 'user_id', undefined)
    } catch (ga4UserIdError) {
      console.error('Failed to clear GA4 user_id:', ga4UserIdError)
    }

    // Track logout event before clearing user data
    try {
      analyticsService.clearUser() // This automatically tracks logout event
    } catch (analyticsError) {
      console.error('Failed to track logout event:', analyticsError)
    }

    // Clear client-side state immediately for responsive UI, then call server logout
    setUser(null)
    setIsAuthenticated(false)

    authService.logout().catch((logoutError: unknown) => {
      console.error('Failed to call server logout:', logoutError)
    })

    // Redirect to sign-in page
    window.location.href = '/sign-in'
  }

  const checkPendingInvitation = (): string | null => {
    const pendingToken = sessionStore.get(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
    if (pendingToken) {
      // Clear the token from storage
      sessionStore.remove(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
      return pendingToken
    }
    return null
  }

  const markOnboardingComplete = async () => {
    try {
      const updatedUser = await authService.markOnboardingComplete()
      setUser(updatedUser)
    } catch (error) {
      console.error('Failed to mark onboarding complete:', error)
      throw error
    }
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated,
        login,
        logout,
        isLoading,
        checkPendingInvitation,
        markOnboardingComplete,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

// Re-export useAuth from separate file to satisfy react-refresh/only-export-components rule
// This maintains backwards compatibility for all existing imports
export { useAuth } from './useAuth'
