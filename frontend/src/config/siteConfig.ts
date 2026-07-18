/**
 * Central, env-driven site configuration.
 *
 * All tenant/brand-specific values (brand name, marketing/legal URLs, support
 * email, MCP endpoint, error-type base URI) live here so a self-hoster can
 * rebrand the app entirely through `VITE_*` environment variables without
 * editing component code. Every value has a neutral placeholder default so the
 * app builds and runs out of the box.
 *
 * Values are read through `getEnv` (issue #57): in the combined image the
 * backend injects them at runtime via `/config.js`, so rebranding is a deploy-
 * time env var + restart, not a rebuild; the build-time `import.meta.env` stays
 * the fallback for local dev.
 */
import { getEnv } from '@/lib/runtimeEnv'

/** Returns `value` when it is a non-empty string, otherwise `fallback`. */
function or(value: string | undefined, fallback: string): string {
  return value !== undefined && value !== '' ? value : fallback
}

/** Strips trailing "/" characters (no regex, so no backtracking pathology). */
function stripTrailingSlashes(value: string): string {
  let end = value.length
  while (end > 0 && value.endsWith('/', end)) {
    end -= 1
  }
  return value.slice(0, end)
}

/** Display name of the product/brand. */
export const SITE_NAME = or(getEnv('VITE_SITE_NAME'), 'VibeXP')

/** Legal entity name shown in copyright notices. */
export const SITE_LEGAL_NAME = or(getEnv('VITE_SITE_LEGAL_NAME'), SITE_NAME)

/** Public marketing/home site URL (used for brand links). */
export const SITE_URL = or(getEnv('VITE_SITE_URL'), 'https://example.com')

/** Hostname portion of SITE_URL, for display as a link label. */
export const SITE_DOMAIN = stripTrailingSlashes(
  SITE_URL.replace(/^https?:\/\//, '')
)

/** Terms & Conditions page URL. */
export const TERMS_URL = or(
  getEnv('VITE_TERMS_URL'),
  `${SITE_URL}/terms-and-conditions`
)

/** Privacy Policy page URL. */
export const PRIVACY_URL = or(
  getEnv('VITE_PRIVACY_URL'),
  `${SITE_URL}/privacy-policy`
)

/** Support contact email address. */
export const SUPPORT_EMAIL = or(
  getEnv('VITE_SUPPORT_EMAIL'),
  'support@example.com'
)

/** Absolute URL to the brand logo (used in shared-page OpenGraph tags). */
export const BRAND_LOGO_URL = or(
  getEnv('VITE_BRAND_LOGO_URL'),
  `${SITE_URL}/logo_rounded.png`
)

/**
 * The single, team-agnostic MCP endpoint advertised in client setup snippets.
 * Self-hosters point this at their own backend's MCP route.
 */
export const MCP_ENDPOINT = or(
  getEnv('VITE_MCP_ENDPOINT'),
  'https://connect.example.com/mcp/v1/common'
)

/**
 * Base URI for RFC 9457 problem-detail `type` fields generated client-side as a
 * fallback when the backend omits one. When `VITE_ERROR_TYPE_BASE_URI` is set,
 * generated types are `<base>/errors/<CODE>`; otherwise the RFC 9457 sentinel
 * `about:blank` is used. Not user-facing.
 */
const ERROR_TYPE_BASE_URI = or(getEnv('VITE_ERROR_TYPE_BASE_URI'), '')

/** Builds the RFC 9457 `type` URI for a client-side fallback problem detail. */
export const errorTypeUri = (code: string): string =>
  ERROR_TYPE_BASE_URI
    ? `${stripTrailingSlashes(ERROR_TYPE_BASE_URI)}/errors/${code}`
    : 'about:blank'
