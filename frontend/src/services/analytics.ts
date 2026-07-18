/**
 * Enhanced Analytics Service
 *
 * This service extends the existing GTM functionality to provide a centralized,
 * type-safe, and feature-rich analytics tracking system for the VibeXP application.
 *
 * Features:
 * - Type-safe event tracking
 * - Environment-specific behavior (development/production)
 * - Error handling and recovery
 * - User context management
 * - Development debugging support
 * - Integration with existing GTM setup
 */

import type {
  AnalyticsConfig,
  AnalyticsEvent,
  AnalyticsService,
  GTMDataLayer,
  TrackAuthParams,
  TrackErrorParams,
  TrackEventParams,
  TrackPageParams,
  UserProperties,
} from '../types/analytics'
import { ANALYTICS_EVENTS } from '../types/analytics'
import {
  logValidationResults,
  validateAnalyticsEvent,
  validateTrackAuthParams,
  validateTrackEventParams,
  validateTrackPageParams,
} from '../utils/analyticsValidation'
import { isDevMode } from '../utils/environment'
import { GTM_ENABLED, GTM_ID, trackEvent } from '../utils/gtm'

class EnhancedAnalyticsService implements AnalyticsService {
  private config: AnalyticsConfig
  private userProperties: UserProperties | null = null
  private readonly sessionId: string | null = null

  constructor() {
    // `environment` reflects the build target. NODE_ENV is statically replaced
    // by Vite at build time and resolves to 'test' under Jest, which keeps unit
    // tests exercising the development code path.
    const isDev =
      process.env.NODE_ENV === 'development' || process.env.NODE_ENV === 'test'

    // Console logging is gated on Vite's canonical dev flag (import.meta.env.DEV,
    // accessed via isDevMode) so that no analytics output — and therefore no PII
    // — can ever reach a production browser console, independent of how NODE_ENV
    // is resolved in the bundle.
    const enableConsoleLogging = isDevMode()

    this.config = {
      gtmId: GTM_ID,
      enabled: GTM_ENABLED,
      debug: enableConsoleLogging,
      environment: isDev ? 'development' : 'production',
      enableConsoleLogging,
      enableErrorTracking: true,
    }

    this.sessionId = this.generateSessionId()

    if (this.config.debug) {
      console.log('[Analytics] Service initialized', {
        config: this.config,
        sessionId: this.sessionId,
      })
    }
  }

  /**
   * Configure the analytics service
   */
  configure(config: Partial<AnalyticsConfig>): void {
    this.config = { ...this.config, ...config }

    if (this.config.debug) {
      console.log('[Analytics] Configuration updated', this.config)
    }
  }

  /**
   * Check if analytics is enabled
   */
  isEnabled(): boolean {
    return this.config.enabled && !!window.dataLayer
  }

  /**
   * Get current configuration
   */
  getConfig(): AnalyticsConfig {
    return { ...this.config }
  }

  /**
   * Generate a unique session ID
   */
  generateSessionId(): string {
    return `session_${String(Date.now())}_${Math.random().toString(36).substring(2, 15)}`
  }

  /**
   * Get current page path
   */
  getCurrentPagePath(): string {
    return window.location.pathname + window.location.search
  }

  /**
   * Get current page title
   */
  getCurrentPageTitle(): string {
    return document.title || 'Unknown Page'
  }

  /**
   * Strip PII (email, name, …) from user properties before they are written to
   * the console. Only the non-identifying `user_id` is retained so debug logs
   * never disclose personal data.
   */
  private redactUserProperties(
    properties: UserProperties | null | undefined
  ): { user_id: string } | undefined {
    return properties ? { user_id: properties.user_id } : undefined
  }

  /**
   * Log events to console in development mode
   */
  private logEvent(eventData: GTMDataLayer): void {
    if (this.config.enableConsoleLogging) {
      console.group(`[Analytics] Event: ${eventData.event ?? 'unknown'}`)
      console.log('Event Data:', {
        ...eventData,
        user_properties: this.redactUserProperties(eventData.user_properties),
      })
      console.log(
        'Timestamp:',
        new Date(eventData.timestamp ?? Date.now()).toISOString()
      )
      console.log(
        'User Properties:',
        this.redactUserProperties(this.userProperties)
      )
      console.groupEnd()
    }
  }

  /**
   * Handle analytics errors gracefully
   */
  private handleError(error: Error, context?: string): void {
    const contextSuffix = context ? ` in ${context}` : ''
    const message = `Analytics Error${contextSuffix}: ${error.message}`

    if (this.config.debug) {
      console.error('[Analytics]', message, error)
    }

    // Don't track analytics errors if error tracking is disabled
    if (!this.config.enableErrorTracking) {
      return
    }

    // Prevent infinite loops by checking if this is already an analytics error
    if (context !== 'error tracking') {
      try {
        this.trackError({
          error,
          component: context ?? 'analytics_service',
        })
      } catch (e) {
        console.error('[Analytics] Failed to track error:', e)
      }
    }
  }

  /**
   * Core tracking method - all other methods use this
   */
  track(event: AnalyticsEvent): void {
    if (!this.isEnabled()) {
      if (this.config.debug) {
        console.warn('[Analytics] Tracking disabled or GTM not available')
      }
      return
    }

    try {
      const eventData: GTMDataLayer = {
        ...event, // Spread event properties first
        event: event.event,
        timestamp: event.timestamp,
        page_path: event.page_path,
        page_title: event.page_title ?? this.getCurrentPageTitle(),
        user_properties:
          event.user_properties ?? this.userProperties ?? undefined,
        session_id: this.sessionId ?? this.generateSessionId(),
        environment: this.config.environment,
      }

      // Validate event in development mode
      if (this.config.debug) {
        const validation = validateAnalyticsEvent(event)
        logValidationResults(event.event, validation)

        if (!validation.isValid) {
          console.error(
            '[Analytics] Invalid event will not be sent:',
            validation.errors
          )
          return
        }
      }

      // Log the event in development
      this.logEvent(eventData)

      // Send to GTM using existing utility
      trackEvent(eventData.event ?? 'unknown', eventData)
    } catch (error) {
      this.handleError(error as Error, 'track')
    }
  }

  /**
   * Track custom events with flexible parameters
   */
  trackEvent(params: TrackEventParams): void {
    try {
      // Validate parameters in development mode
      if (this.config.debug) {
        const validation = validateTrackEventParams(params)
        logValidationResults(`trackEvent(${params.event})`, validation)

        if (!validation.isValid) {
          console.error(
            '[Analytics] Invalid trackEvent parameters:',
            validation.errors
          )
          return
        }
      }

      const event: AnalyticsEvent = {
        event: params.event as AnalyticsEvent['event'], // Use proper type
        timestamp: Date.now(),
        page_path: this.getCurrentPagePath(),
        page_title: this.getCurrentPageTitle(),
        user_properties:
          params.userProperties ?? this.userProperties ?? undefined,
        environment: this.config.environment,
        ...params.properties,
      } as AnalyticsEvent

      this.track(event)
    } catch (error) {
      this.handleError(error as Error, 'trackEvent')
    }
  }

  /**
   * Track page views with automatic path and title detection
   */
  trackPage(params: TrackPageParams): void {
    try {
      // Validate parameters in development mode
      if (this.config.debug) {
        const validation = validateTrackPageParams(params)
        logValidationResults(`trackPage(${params.path})`, validation)

        if (!validation.isValid) {
          console.error(
            '[Analytics] Invalid trackPage parameters:',
            validation.errors
          )
          return
        }
      }

      const event: AnalyticsEvent = {
        event: ANALYTICS_EVENTS.PAGE_VIEW,
        timestamp: Date.now(),
        page_path: params.path,
        page_title: params.title,
        referrer: params.referrer ?? document.referrer,
        user_properties:
          params.userProperties ?? this.userProperties ?? undefined,
        environment: this.config.environment,
      }

      this.track(event)
    } catch (error) {
      this.handleError(error as Error, 'trackPage')
    }
  }

  /**
   * Track authentication events
   */
  trackAuth(params: TrackAuthParams): void {
    try {
      // Validate parameters in development mode
      if (this.config.debug) {
        const validation = validateTrackAuthParams(params)
        logValidationResults(`trackAuth(${params.eventType})`, validation)

        if (!validation.isValid) {
          console.error(
            '[Analytics] Invalid trackAuth parameters:',
            validation.errors
          )
          return
        }
      }

      let eventName: string

      switch (params.eventType) {
        case 'signin_page_view':
          eventName = ANALYTICS_EVENTS.USER_SIGNIN_PAGE_VIEW
          break
        case 'signed_in':
          eventName = ANALYTICS_EVENTS.USER_SIGNED_IN
          break
        case 'signed_in_first_time':
          eventName = ANALYTICS_EVENTS.USER_SIGNED_IN_FIRST_TIME
          break
        case 'logged_out':
          eventName = ANALYTICS_EVENTS.USER_LOGGED_OUT
          break
        default:
          throw new Error(
            `Unknown auth event type: ${String(params.eventType)}`
          )
      }

      const event: AnalyticsEvent = {
        event: eventName as AnalyticsEvent['event'], // Use proper type
        timestamp: Date.now(),
        page_path: this.getCurrentPagePath(),
        page_title: this.getCurrentPageTitle(),
        user_properties:
          params.userProperties ?? this.userProperties ?? undefined,
        environment: this.config.environment,
      } as AnalyticsEvent

      this.track(event)
    } catch (error) {
      this.handleError(error as Error, 'trackAuth')
    }
  }

  /**
   * Track JavaScript errors and analytics errors
   */
  trackError(params: TrackErrorParams): void {
    try {
      const event: AnalyticsEvent = {
        event: ANALYTICS_EVENTS.JAVASCRIPT_ERROR,
        timestamp: Date.now(),
        page_path: this.getCurrentPagePath(),
        page_title: this.getCurrentPageTitle(),
        error_message: params.error.message,
        error_stack: params.error.stack,
        error_component: params.component,
        user_properties: this.userProperties ?? undefined,
        environment: this.config.environment,
        ...params.additionalInfo,
      }

      this.track(event)
    } catch (error) {
      // Don't call handleError here to prevent infinite loops
      if (this.config.debug) {
        console.error('[Analytics] Failed to track error:', error)
      }
    }
  }

  /**
   * Set user identification and properties
   */
  identify(userProperties: UserProperties): void {
    try {
      this.userProperties = userProperties

      if (this.config.debug) {
        console.log(
          '[Analytics] User identified:',
          this.redactUserProperties(userProperties)
        )
      }

      // Update GTM data layer with user information
      if (this.isEnabled()) {
        window.dataLayer.push({
          user_id: userProperties.user_id,
          user_properties: userProperties,
          event: 'user_identified',
        })
      }
    } catch (error) {
      this.handleError(error as Error, 'identify')
    }
  }

  /**
   * Update user properties without full re-identification
   */
  setUserProperties(properties: Partial<UserProperties>): void {
    try {
      if (this.userProperties) {
        this.userProperties = { ...this.userProperties, ...properties }

        if (this.config.debug) {
          console.log(
            '[Analytics] User properties updated:',
            this.redactUserProperties(this.userProperties)
          )
        }

        // Update GTM data layer
        if (this.isEnabled()) {
          window.dataLayer.push({
            user_properties: this.userProperties,
            event: 'user_properties_updated',
          })
        }
      } else if (this.config.debug) {
        console.warn(
          '[Analytics] Cannot update user properties - user not identified'
        )
      }
    } catch (error) {
      this.handleError(error as Error, 'setUserProperties')
    }
  }

  /**
   * Clear user identification (for logout)
   */
  clearUser(): void {
    try {
      const previousUser = this.userProperties

      this.userProperties = null

      if (this.config.debug) {
        console.log('[Analytics] User cleared')
      }

      // Track logout event before clearing
      if (previousUser) {
        this.trackAuth({
          eventType: 'logged_out',
          userProperties: previousUser,
        })
      }

      // Clear user data from GTM data layer
      if (this.isEnabled()) {
        window.dataLayer.push({
          user_id: undefined,
          user_properties: undefined,
          event: 'user_cleared',
        })
      }
    } catch (error) {
      this.handleError(error as Error, 'clearUser')
    }
  }
}

// Create singleton instance
const analyticsService = new EnhancedAnalyticsService()

// Setup global error tracking
if (analyticsService.getConfig().enableErrorTracking) {
  window.addEventListener('error', event => {
    analyticsService.trackError({
      error: (event.error as Error | null) ?? new Error(event.message),
      component: 'global_error_handler',
      additionalInfo: {
        filename: event.filename,
        lineno: event.lineno,
        colno: event.colno,
      },
    })
  })

  window.addEventListener('unhandledrejection', event => {
    analyticsService.trackError({
      error:
        event.reason instanceof Error
          ? event.reason
          : new Error(String(event.reason)),
      component: 'unhandled_promise_rejection',
    })
  })
}

export { analyticsService }
export default analyticsService
