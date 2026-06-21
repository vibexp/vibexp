/**
 * Sentry beforeSend filter logic — pure TypeScript, no Vite or React Router dependencies.
 *
 * Kept in a separate file so it can be directly imported in Jest tests without
 * triggering the import.meta.env / React Router module resolution issues present
 * in sentry.ts.
 */

/**
 * Minimal types for the beforeSend callback signature.
 * These match @sentry/core's ErrorEvent and EventHint without importing Sentry.
 */
export type SentryFilterEvent = Record<string, unknown>

export interface SentryFilterHint {
  originalException?: unknown
}

/**
 * Sentry beforeSend filter — called for every event before it is sent to Sentry.
 * Returns null to drop the event, or the event to send it.
 *
 * Filters out known benign errors caused by third-party DOM manipulation so they
 * do not create false-positive noise in the Sentry dashboard.
 */
export function sentryBeforeSend(
  event: SentryFilterEvent,
  hint: SentryFilterHint
): SentryFilterEvent | null {
  const error = hint.originalException

  // Filter out ResizeObserver errors (benign browser noise).
  // Also filtered via ignoreErrors array in Sentry.init() as belt-and-suspenders.
  if (error instanceof Error && error.message.includes('ResizeObserver')) {
    return null
  }

  // Filter out DOM NotFoundError exceptions caused by third-party scripts
  // (browser extensions, Google Translate, GTM) manipulating the DOM
  // outside React's control. These are not application crashes — they are
  // handled errors that create false-positive noise in Sentry.
  // See: VIBEXP-FRONTEND-JS-7 (removeChild), VIBEXP-FRONTEND-JS-8 (object not found)
  if (
    error instanceof Error &&
    error.name === 'NotFoundError' &&
    (error.message.includes('removeChild') ||
      error.message.includes('The object can not be found here'))
  ) {
    return null
  }

  return event
}
