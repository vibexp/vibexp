/**
 * useAnalytics Hook
 *
 * Main React hook for component-level event tracking. Provides a convenient
 * interface for components to track analytics events without directly
 * importing the analytics service.
 *
 * Features:
 * - Memoized tracking functions to prevent unnecessary re-renders
 * - Type-safe event tracking
 * - Error boundary integration
 * - Automatic user context management
 */

import { useCallback, useMemo } from 'react'

import { useAuth } from '../contexts/AuthContext'
import { analyticsService } from '../services/analytics'
import type {
  AnalyticsEvent,
  TrackAuthParams,
  TrackErrorParams,
  TrackEventParams,
  TrackPageParams,
  UseAnalyticsReturn,
  UserProperties,
} from '../types/analytics'

export function useAnalytics(): UseAnalyticsReturn {
  const { user, isAuthenticated } = useAuth()

  // Convert user data to analytics user properties
  const userProperties: UserProperties | undefined = useMemo(() => {
    if (!isAuthenticated || !user) {
      return undefined
    }

    return {
      user_id: user.id,
      email: user.email,
      name: user.name,
      signup_date: user.created_at,
      avatar_url: user.avatar_url ?? null,
      created_at: user.created_at,
    }
  }, [isAuthenticated, user])

  // Memoized tracking functions to prevent unnecessary re-renders
  const track = useCallback(
    (event: AnalyticsEvent) => {
      try {
        // Automatically include user properties if available
        const eventWithUser: AnalyticsEvent = {
          ...event,
          user_properties: event.user_properties ?? userProperties,
        }

        analyticsService.track(eventWithUser)
      } catch (error) {
        console.error('[useAnalytics] Error tracking event:', error)
      }
    },
    [userProperties]
  )

  const trackEvent = useCallback(
    (params: TrackEventParams) => {
      try {
        analyticsService.trackEvent({
          ...params,
          userProperties: params.userProperties ?? userProperties,
        })
      } catch (error) {
        console.error('[useAnalytics] Error tracking custom event:', error)
      }
    },
    [userProperties]
  )

  const trackPage = useCallback(
    (params: TrackPageParams) => {
      try {
        analyticsService.trackPage({
          ...params,
          userProperties: params.userProperties ?? userProperties,
        })
      } catch (error) {
        console.error('[useAnalytics] Error tracking page:', error)
      }
    },
    [userProperties]
  )

  const trackAuth = useCallback(
    (params: TrackAuthParams) => {
      try {
        analyticsService.trackAuth({
          ...params,
          userProperties: params.userProperties ?? userProperties,
        })
      } catch (error) {
        console.error('[useAnalytics] Error tracking auth event:', error)
      }
    },
    [userProperties]
  )

  const trackError = useCallback((params: TrackErrorParams) => {
    try {
      analyticsService.trackError(params)
    } catch (error) {
      console.error('[useAnalytics] Error tracking error:', error)
    }
  }, [])

  const identify = useCallback((properties: UserProperties) => {
    try {
      analyticsService.identify(properties)
    } catch (error) {
      console.error('[useAnalytics] Error identifying user:', error)
    }
  }, [])

  const isEnabled = useMemo(() => {
    return analyticsService.isEnabled()
  }, [])

  return {
    track,
    trackEvent,
    trackPage,
    trackAuth,
    trackError,
    identify,
    isEnabled,
  }
}
