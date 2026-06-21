/**
 * usePageTracking Hook
 *
 * React hook for automatic page view tracking with React Router integration.
 * Automatically tracks page views when the route changes and provides manual
 * page tracking capabilities.
 *
 * Features:
 * - Automatic page view tracking on route changes
 * - Manual page view tracking
 * - Debouncing to prevent duplicate events
 * - Integration with React Router location changes
 * - User context aware tracking
 */

import React, { useCallback, useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'

import { STORAGE_KEYS } from '../constants/storageKeys'
import type { UsePageTrackingReturn } from '../types/analytics'
import { sessionStore } from '../utils/storage'
import { useAnalytics } from './useAnalytics'

export function usePageTracking(
  options: {
    enableAutoTracking?: boolean
    debounceMs?: number
  } = {}
): UsePageTrackingReturn {
  const { enableAutoTracking = true, debounceMs = 100 } = options
  const location = useLocation()
  const { trackPage, isEnabled } = useAnalytics()
  const lastTrackedPath = useRef<string>('')
  const debounceTimer = useRef<NodeJS.Timeout | null>(null)

  // Get page title for current location
  const getCurrentTitle = useCallback((): string => {
    return document.title || 'Unknown Page'
  }, [])

  // Get referrer information
  const getReferrer = useCallback((): string | undefined => {
    // Get referrer from document or session storage for SPA navigation
    const documentReferrer = document.referrer
    const sessionReferrer = sessionStore.get(STORAGE_KEYS.ANALYTICS_REFERRER)

    if (documentReferrer && documentReferrer !== window.location.href) {
      return documentReferrer
    }

    if (sessionReferrer) {
      return sessionReferrer
    }

    return undefined
  }, [])

  // Track current page view
  const trackCurrentPage = useCallback((): void => {
    if (!isEnabled) {
      return
    }

    const currentPath = location.pathname + location.search
    const currentTitle = getCurrentTitle()
    const referrer = getReferrer()

    // Store current page as referrer for next navigation
    sessionStore.set(STORAGE_KEYS.ANALYTICS_REFERRER, window.location.href)

    trackPage({
      path: currentPath,
      title: currentTitle,
      referrer,
    })

    lastTrackedPath.current = currentPath
  }, [
    location.pathname,
    location.search,
    isEnabled,
    trackPage,
    getCurrentTitle,
    getReferrer,
  ])

  // Manual page view tracking with custom path and title
  const trackPageView = useCallback(
    (path?: string, title?: string): void => {
      if (!isEnabled) {
        return
      }

      const targetPath = path ?? location.pathname + location.search
      const targetTitle = title ?? getCurrentTitle()
      const referrer = getReferrer()

      trackPage({
        path: targetPath,
        title: targetTitle,
        referrer,
      })

      if (!path) {
        lastTrackedPath.current = targetPath
      }
    },
    [
      location.pathname,
      location.search,
      isEnabled,
      trackPage,
      getCurrentTitle,
      getReferrer,
    ]
  )

  // Handle route changes with debouncing
  const handleRouteChange = useCallback(() => {
    if (!enableAutoTracking || !isEnabled) {
      return
    }

    const currentPath = location.pathname + location.search

    // Skip if it's the same path (prevent duplicate tracking)
    if (currentPath === lastTrackedPath.current) {
      return
    }

    // Clear existing debounce timer
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current)
    }

    // Debounce the tracking call
    debounceTimer.current = setTimeout(() => {
      trackCurrentPage()
    }, debounceMs)
  }, [
    enableAutoTracking,
    isEnabled,
    location.pathname,
    location.search,
    trackCurrentPage,
    debounceMs,
  ])

  // Effect to handle automatic page tracking on route changes
  useEffect(() => {
    handleRouteChange()

    // Cleanup debounce timer on unmount
    return () => {
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current)
      }
    }
  }, [handleRouteChange])

  // Effect to track initial page view on mount
  useEffect(() => {
    if (enableAutoTracking && isEnabled && !lastTrackedPath.current) {
      // Small delay to ensure the page is fully loaded
      const initialTimer = setTimeout(() => {
        trackCurrentPage()
      }, 50)

      return () => {
        clearTimeout(initialTimer)
      }
    }
  }, [enableAutoTracking, isEnabled, trackCurrentPage])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current)
        debounceTimer.current = null
      }
    }
  }, [])

  return {
    trackCurrentPage,
    trackPageView,
  }
}

/**
 * Higher-order component for automatic page tracking
 * Useful for wrapping the entire app or specific route components
 */
export function withPageTracking<P extends object>(
  Component: React.ComponentType<P>,
  options?: {
    enableAutoTracking?: boolean
    debounceMs?: number
  }
): React.ComponentType<P> {
  return function PageTrackingWrapper(props: P) {
    usePageTracking(options)
    return <Component {...props} />
  }
}
