/**
 * Central, env-driven site configuration.
 *
 * All tenant/brand-specific values (brand name, marketing/legal URLs, support
 * email, MCP endpoint, error-type base URI) live here so a self-hoster can
 * rebrand the app entirely through `VITE_*` environment variables without
 * editing component code. Every value has a neutral placeholder default so the
 * app builds and runs out of the box.
 */

/** Returns `value` when it is a non-empty string, otherwise `fallback`. */
function or(value: string | undefined, fallback: string): string {
  return value !== undefined && value !== '' ? value : fallback
}

// `import.meta.env` is absent under some test runners; default to an empty bag
// so every lookup below falls back to its neutral placeholder.
const env: Partial<ImportMetaEnv> =
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  import.meta?.env ?? {}

/** Display name of the product/brand. */
export const SITE_NAME = or(env.VITE_SITE_NAME, 'VibeXP')

/** Legal entity name shown in copyright notices. */
export const SITE_LEGAL_NAME = or(env.VITE_SITE_LEGAL_NAME, SITE_NAME)

/** Public marketing/home site URL (used for brand links). */
export const SITE_URL = or(env.VITE_SITE_URL, 'https://example.com')

/** Hostname portion of SITE_URL, for display as a link label. */
export const SITE_DOMAIN = SITE_URL.replace(/^https?:\/\//, '').replace(
  /\/+$/,
  ''
)

/** Terms & Conditions page URL. */
export const TERMS_URL = or(env.VITE_TERMS_URL, `${SITE_URL}/terms-and-conditions`)

/** Privacy Policy page URL. */
export const PRIVACY_URL = or(env.VITE_PRIVACY_URL, `${SITE_URL}/privacy-policy`)

/** Support contact email address. */
export const SUPPORT_EMAIL = or(env.VITE_SUPPORT_EMAIL, 'support@example.com')

/** Absolute URL to the brand logo (used in shared-page OpenGraph tags). */
export const BRAND_LOGO_URL = or(
  env.VITE_BRAND_LOGO_URL,
  `${SITE_URL}/logo_rounded.png`
)

/**
 * The single, team-agnostic MCP endpoint advertised in client setup snippets.
 * Self-hosters point this at their own backend's MCP route.
 */
export const MCP_ENDPOINT = or(
  env.VITE_MCP_ENDPOINT,
  'https://connect.example.com/mcp/v1/common'
)

/**
 * Base URI for RFC 9457 problem-detail `type` fields generated client-side as a
 * fallback when the backend omits one. When `VITE_ERROR_TYPE_BASE_URI` is set,
 * generated types are `<base>/errors/<CODE>`; otherwise the RFC 9457 sentinel
 * `about:blank` is used. Not user-facing.
 */
const ERROR_TYPE_BASE_URI = or(env.VITE_ERROR_TYPE_BASE_URI, '')

/** Builds the RFC 9457 `type` URI for a client-side fallback problem detail. */
export const errorTypeUri = (code: string): string =>
  ERROR_TYPE_BASE_URI
    ? `${ERROR_TYPE_BASE_URI.replace(/\/+$/, '')}/errors/${code}`
    : 'about:blank'
