/**
 * Sentry Error Tracking Configuration
 *
 * Initializes Sentry for frontend error monitoring and tracking.
 * Provides proper error tracking, performance monitoring, and user context.
 */

import * as Sentry from '@sentry/react'

// Import filter function from the Vite-independent module (no import.meta.env
// or React Router dependencies) so it can be directly tested in Jest.
// Re-exported so callers can import from either location.
import { sentryBeforeSend } from './sentry-filters'
export { sentryBeforeSend }

/**
 * Initialize Sentry with configuration
 * Should be called as early as possible in the application lifecycle
 */
export function initSentry() {
  // Only initialize in production (when VITE_GTM_ENABLED=true is set)
  const isProduction = import.meta.env.PROD
  const gtmEnabled = import.meta.env.VITE_GTM_ENABLED === 'true'
  const sentryDsn = import.meta.env.VITE_SENTRY_DSN as string | undefined
  const environment = isProduction ? 'production' : import.meta.env.MODE

  // Only initialize if in production mode (same as GTM)
  if (!isProduction || !gtmEnabled) {
    console.log('[Sentry] Disabled in development mode')
    return
  }

  // Don't initialize if no DSN provided
  if (!sentryDsn) {
    console.warn(
      '[Sentry] No DSN provided in production, error tracking disabled'
    )
    return
  }

  Sentry.init({
    dsn: sentryDsn,

    // Environment configuration
    environment,

    // Enable structured logging
    enableLogs: true,

    // Integrations
    integrations: [
      // Browser tracing for performance monitoring
      Sentry.browserTracingIntegration(),

      // React-specific integrations
      Sentry.reactRouterV6BrowserTracingIntegration({
        useEffect: React.useEffect,
        useLocation,
        useNavigationType,
        createRoutesFromChildren,
        matchRoutes,
      }),

      // Console logging integration - send console.log, console.warn, and console.error to Sentry
      Sentry.consoleLoggingIntegration({
        levels: ['warn', 'error'], // Only log warnings and errors, not all logs
      }),

      // Replay integration for session replay (optional)
      Sentry.replayIntegration({
        maskAllText: true,
        blockAllMedia: true,
      }),
    ],

    // Performance Monitoring - 10% sampling (this code only runs in production due to early return above)
    tracesSampleRate: 0.1,

    // Session Replay
    replaysSessionSampleRate: 0.1, // 10% of sessions
    replaysOnErrorSampleRate: 1.0, // 100% of sessions with errors

    // Release tracking
    release:
      (import.meta.env.VITE_APP_VERSION as string | undefined) ?? 'unknown',

    // Additional configuration
    // Double cast required: sentryBeforeSend uses framework-agnostic types in
    // sentry-filters.ts; the runtime behaviour is identical to Sentry.ErrorEvent.
    beforeSend:
      sentryBeforeSend as unknown as Sentry.BrowserOptions['beforeSend'],

    // Ignore specific errors
    ignoreErrors: [
      // Browser extensions
      'top.GLOBALS',
      'chrome-extension://',
      'moz-extension://',

      // Network errors that are expected
      'NetworkError',
      'Failed to fetch',
      'Network request failed',

      // ResizeObserver errors (benign)
      'ResizeObserver loop',
    ],
  })

  // Add release metadata to Sentry context
  Sentry.setContext('release_metadata', {
    sha: import.meta.env.VITE_RELEASE_SHA || 'dev',
    date: import.meta.env.VITE_RELEASE_DATE || 'unknown',
  })
}

/**
 * Set user context for Sentry
 */
export function setSentryUser(user: {
  id: string
  email?: string
  username?: string
}) {
  Sentry.setUser({
    id: user.id,
    email: user.email,
    username: user.username,
  })
}

/**
 * Clear user context (on logout)
 */
export function clearSentryUser() {
  Sentry.setUser(null)
}

/**
 * Add custom context to Sentry
 */
export function setSentryContext(
  key: string,
  context: Record<string, unknown>
) {
  Sentry.setContext(key, context)
}

/**
 * Add breadcrumb for debugging
 */
export function addSentryBreadcrumb(
  message: string,
  category?: string,
  level?: Sentry.SeverityLevel,
  data?: Record<string, unknown>
) {
  Sentry.addBreadcrumb({
    message,
    category: category ?? 'custom',
    level: level ?? 'info',
    data,
  })
}

/**
 * Manually capture an exception
 */
export function captureException(
  error: Error,
  context?: Record<string, unknown>
) {
  Sentry.captureException(error, {
    extra: context,
  })
}

/**
 * Manually capture a message
 */
export function captureMessage(
  message: string,
  level?: Sentry.SeverityLevel,
  context?: Record<string, unknown>
) {
  Sentry.captureMessage(message, {
    level: level ?? 'info',
    extra: context,
  })
}

/**
 * Structured Logger (Sentry best practice)
 * Use logger.fmt template literal for variables in logs
 */
export const logger = {
  trace: (message: string, data?: Record<string, unknown>) => {
    console.trace(message, data)
  },
  debug: (message: string, data?: Record<string, unknown>) => {
    console.debug(message, data)
  },
  info: (message: string, data?: Record<string, unknown>) => {
    console.info(message, data)
  },
  warn: (message: string, data?: Record<string, unknown>) => {
    console.warn(message, data)
  },
  error: (message: string, data?: Record<string, unknown>) => {
    console.error(message, data)
  },
  fatal: (message: string, data?: Record<string, unknown>) => {
    console.error('[FATAL]', message, data)
    // Fatal errors should be captured to Sentry
    Sentry.captureMessage(message, {
      level: 'fatal',
      extra: data,
    })
  },
  // Template literal helper for structured logs
  fmt: (strings: TemplateStringsArray, ...values: unknown[]) => {
    return strings.reduce<{ result: string; valueIndex: number }>(
      (acc, str) => {
        const val = values[acc.valueIndex]
        const valStr =
          val !== undefined
            ? typeof val === 'string'
              ? val
              : JSON.stringify(val)
            : ''
        return {
          result: acc.result + str + valStr,
          valueIndex: acc.valueIndex + 1,
        }
      },
      { result: '', valueIndex: 0 }
    ).result
  },
}

/**
 * Create a custom span for UI interactions
 * Example: trackUIInteraction('ui.click', 'Submit Form Button', { formId: 'contact' })
 */
export function trackUIInteraction(
  operation: string,
  name: string,
  attributes?: Record<string, unknown>,
  callback?: () => void | Promise<void>
) {
  return Sentry.startSpan(
    {
      op: operation,
      name,
    },
    async span => {
      // Add attributes to the span
      if (attributes) {
        Object.entries(attributes).forEach(([key, value]) => {
          span.setAttribute(
            key,
            value as Parameters<typeof span.setAttribute>[1]
          )
        })
      }

      // Execute callback if provided
      if (callback) {
        await callback()
      }
    }
  )
}

/**
 * Create a custom span for API calls
 * Example: trackAPICall('GET /api/users', async () => { return await fetch(...) })
 */
export async function trackAPICall<T>(
  name: string,
  callback: () => Promise<T>,
  attributes?: Record<string, unknown>
): Promise<T> {
  return Sentry.startSpan(
    {
      op: 'http.client',
      name,
    },
    async span => {
      // Add attributes to the span
      if (attributes) {
        Object.entries(attributes).forEach(([key, value]) => {
          span.setAttribute(
            key,
            value as Parameters<typeof span.setAttribute>[1]
          )
        })
      }

      try {
        const result = await callback()
        span.setAttribute('http.status_code', 200)
        return result
      } catch (error) {
        span.setAttribute(
          'http.status_code',
          error instanceof Error ? 500 : 400
        )
        throw error
      }
    }
  )
}

/**
 * Create a custom span for function execution
 * Example: trackFunction('database.query', 'Fetch user by ID', async () => { return await db.query(...) })
 */
export async function trackFunction<T>(
  operation: string,
  name: string,
  callback: () => Promise<T> | T,
  attributes?: Record<string, unknown>
): Promise<T> {
  return Sentry.startSpan(
    {
      op: operation,
      name,
    },
    async span => {
      // Add attributes to the span
      if (attributes) {
        Object.entries(attributes).forEach(([key, value]) => {
          span.setAttribute(
            key,
            value as Parameters<typeof span.setAttribute>[1]
          )
        })
      }

      return await callback()
    }
  )
}

// Re-export Sentry for advanced usage
export { Sentry }

// Import React Router v6 hooks for integration
import React from 'react'
import {
  createRoutesFromChildren,
  matchRoutes,
  useLocation,
  useNavigationType,
} from 'react-router-dom'
