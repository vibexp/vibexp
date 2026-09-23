/**
 * Environment detection utility for API base URL
 * Handles both Vite builds and Jest test environments
 */

/**
 * Whether the app is running in Vite's development mode.
 *
 * Centralizes the `import.meta.env.DEV` access so consumers (e.g. analytics
 * console logging) stay testable: this module is mocked under Jest, where
 * `import.meta` is unavailable, while real Vite builds get the canonical flag.
 * Returns false in production builds — the safe default for gating debug output.
 */
export const isDevMode = (): boolean => import.meta.env.DEV

/**
 * Resolves the API base URL the app talks to.
 *
 * Fully env-driven via `VITE_API_BASE_URL` (set per deployment). When it is not
 * set, dev builds fall back to the local backend and all other cases fall back
 * to an empty string, which makes requests same-origin relative — there is no
 * hardcoded production host, so self-hosters configure the origin entirely
 * through the environment.
 */
export const getApiBaseUrl = (): string => {
  // Test environment (Jest): driven by VITE_API_BASE_URL injected in jest.config.
  if (typeof process !== 'undefined' && process.env.NODE_ENV === 'test') {
    return process.env.VITE_API_BASE_URL ?? ''
  }

  // For Vite builds, use standard import.meta.env access. Typed as possibly
  // undefined because it is absent outside a Vite transform.
  const viteEnv = import.meta.env as ImportMetaEnv | undefined
  if (viteEnv) {
    // First check for explicit VITE_API_BASE_URL
    if (viteEnv.VITE_API_BASE_URL) {
      return viteEnv.VITE_API_BASE_URL
    }

    // Then check development mode
    if (viteEnv.DEV) {
      return 'http://localhost:8080/api/v1'
    }
  }

  // Neutral default: same-origin relative requests. No hardcoded host.
  return ''
}

/**
 * Origin of the backend that serves the API, MCP endpoint and OpenAPI spec.
 *
 * Derived from `getApiBaseUrl()`, resolved against the browsing origin: a
 * relative or empty base (the combined image, #61, where the SPA and the API
 * share one origin) yields the browsing origin, while an absolute base (local
 * dev on `:5173` talking to `:8080`, or any split deployment) yields the
 * backend's. Use it for URLs that must reach the backend rather than the SPA
 * (#1129). A malformed base falls back to the browsing origin; outside a DOM
 * there is no origin to resolve against, so it returns an empty string.
 */
export const getBackendOrigin = (): string => {
  if (typeof window === 'undefined') {
    return ''
  }
  const browsingOrigin = window.location.origin
  try {
    return new URL(getApiBaseUrl() || '/', browsingOrigin).origin
  } catch {
    return browsingOrigin
  }
}
