// Google Tag Manager / gtag glue.
//
// `dataLayer` and `gtag` are OPTIONAL: nothing defines them unless an operator
// configured a GTM container and `initializeGTM()` installed them. Every read
// below (and in `services/analytics.ts`) must therefore guard rather than
// assume — VibeXP ships with no analytics at all by default.
import { getEnv } from '@/lib/runtimeEnv'

declare global {
  interface Window {
    dataLayer?: Record<string, unknown>[]
    gtag?: (...args: unknown[]) => void
  }
}

// Analytics config is read at runtime (issue #57): the backend injects
// VITE_GTM_* via /config.js, with the build-time import.meta.env as fallback.
// GTM is opt-in and ID-driven — configuring a container ID is the whole
// opt-in, so the neutral default (unset) keeps analytics off.
export const GTM_ID = getEnv('VITE_GTM_ID') ?? ''
export const GTM_ENABLED = GTM_ID !== ''
export const GA4_MEASUREMENT_ID = getEnv('VITE_GA4_MEASUREMENT_ID') ?? ''

export const initializeGTM = () => {
  if (!GTM_ENABLED) {
    return
  }

  // Install the globals GTM expects. Nothing else creates them — the app ships
  // no inline bootstrap — so this must run before the first dataLayer push.
  window.dataLayer ??= []
  window.gtag ??= function gtag(...args: unknown[]) {
    window.dataLayer?.push(args as unknown as Record<string, unknown>)
  }

  window.dataLayer.push({
    'gtm.start': Date.now(),
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

// Push a raw dataLayer payload, no-op when no dataLayer exists (i.e. no GTM
// container is configured). Use this for GA4's own reserved event names —
// `trackEvent` below namespaces everything it pushes.
export const pushDataLayerEvent = (payload: Record<string, unknown>) => {
  if (!Array.isArray(window.dataLayer)) {
    return
  }

  window.dataLayer.push(payload)
}

// Associate subsequent GA4 hits with a user (pass undefined to clear).
// No-op when no GTM container is configured.
export const setGA4UserId = (userId?: string) => {
  if (typeof window.gtag !== 'function') {
    return
  }

  window.gtag('set', 'user_id', userId)
}

// Helper function to track custom events
export const trackEvent = (
  eventName: string,
  parameters?: Record<string, unknown>
) => {
  if (!GTM_ENABLED) {
    return
  }

  // No dataLayer means GTM never initialized (or we are under test) — the
  // declared type is optional precisely because that is a normal state.
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

    const gtag = window.gtag
    if (typeof gtag !== 'function') {
      resolve('')
      return
    }

    // Set a timeout to prevent hanging if callback never fires
    const timeoutId = setTimeout(() => {
      console.warn('GA4 client_id retrieval timed out after 2 seconds')
      resolve('')
    }, 2000)

    try {
      gtag('get', GA4_MEASUREMENT_ID, 'client_id', (clientId: string) => {
        clearTimeout(timeoutId)
        resolve(clientId || '')
      })
    } catch (error) {
      clearTimeout(timeoutId)
      console.error('Error getting GA4 client_id:', error)
      resolve('')
    }
  })
}
