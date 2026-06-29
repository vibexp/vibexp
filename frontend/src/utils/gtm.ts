// Google Analytics gtag function type definitions
// gtag and dataLayer are always defined in index.html before any other scripts load
import { getEnv } from '@/lib/runtimeEnv'

declare global {
  interface Window {
    dataLayer: Record<string, unknown>[]
    gtag: (...args: unknown[]) => void
  }
}

// Analytics config is read at runtime (issue #57): the backend injects
// VITE_GTM_* via /config.js, with the build-time import.meta.env as fallback.
// GTM is opt-in — enabled only when VITE_GTM_ENABLED is exactly "true" — so the
// neutral default (unset) keeps analytics off.
export const GTM_ID = getEnv('VITE_GTM_ID') ?? ''
export const GTM_ENABLED = getEnv('VITE_GTM_ENABLED') === 'true'
export const GA4_MEASUREMENT_ID = getEnv('VITE_GA4_MEASUREMENT_ID') ?? ''

export const initializeGTM = () => {
  if (!GTM_ENABLED || !GTM_ID) {
    console.log('GTM is disabled or GTM_ID is not provided')
    return
  }

  // dataLayer is already initialized in index.html with consent defaults
  // Just push the GTM start event
  window.dataLayer.push({
    'gtm.start': new Date().getTime(),
    event: 'gtm.js',
  })

  // Add GTM script
  const script = document.createElement('script')
  script.async = true
  script.src = `https://www.googletagmanager.com/gtm.js?id=${GTM_ID}`

  // Try to insert GTM script before first script tag, otherwise append to head
  const scripts = document.getElementsByTagName('script')
  const firstScript = scripts.length > 0 ? scripts[0] : null

  if (firstScript) {
    firstScript.parentNode?.insertBefore(script, firstScript)
  } else {
    document.head.appendChild(script)
  }
}

// Helper function to track custom events
export const trackEvent = (
  eventName: string,
  parameters?: Record<string, unknown>
) => {
  if (!GTM_ENABLED) {
    return
  }

  // Defensive check: dataLayer is always initialized in index.html,
  // but we check anyway to prevent errors in edge cases (e.g., tests)
  // Using Array.isArray avoids TypeScript's "always truthy" warning
  if (!Array.isArray(window.dataLayer)) {
    return
  }

  const prefixedEventName = `vx_frontend_${eventName}`

  // Destructure to exclude 'event' property from parameters to prevent overwriting the prefixed event name
  // We use _event prefix to indicate intentionally unused
  const { event: _event, ...otherParameters } = parameters ?? {}

  window.dataLayer.push({
    event: prefixedEventName,
    ...otherParameters,
  })
}

// Helper function to get GA4 client_id for attribution linking
// Includes timeout to prevent hanging if GA4 hasn't initialized
export const getGA4ClientId = (): Promise<string> => {
  return new Promise(resolve => {
    if (!GTM_ENABLED || !GA4_MEASUREMENT_ID) {
      resolve('')
      return
    }

    // Set a timeout to prevent hanging if callback never fires
    const timeoutId = setTimeout(() => {
      console.warn('GA4 client_id retrieval timed out after 2 seconds')
      resolve('')
    }, 2000)

    try {
      window.gtag(
        'get',
        GA4_MEASUREMENT_ID,
        'client_id',
        (clientId: string) => {
          clearTimeout(timeoutId)
          resolve(clientId || '')
        }
      )
    } catch (error) {
      clearTimeout(timeoutId)
      console.error('Error getting GA4 client_id:', error)
      resolve('')
    }
  })
}
