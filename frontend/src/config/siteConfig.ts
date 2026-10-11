/**
 * Central, env-driven site configuration.
 *
 * What is left here is deployment plumbing: the MCP endpoint advertised in
 * client setup snippets and the base URI of client-generated error types.
 * Company branding (name, logo, support and legal links) is NOT configured
 * here any more: an instance admin edits it as site info, and components read
 * it through `useSiteInfo` (`features/site-info`, epic #1352).
 *
 * Values are read through `getEnv` (issue #57): in the combined image the
 * backend injects them at runtime via `/config.js`, so changing one is a
 * deploy-time env var + restart, not a rebuild; the build-time
 * `import.meta.env` stays the fallback for local dev.
 */
import { getEnv } from '@/lib/runtimeEnv'
import { getBackendOrigin } from '@/utils/environment'

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

/**
 * Default MCP endpoint for the instance currently being browsed.
 *
 * The backend mounts the team-agnostic MCP handler at the fixed path
 * `/mcp/v1/common` on its own origin, which `getBackendOrigin()` resolves from
 * the API base URL: in the combined image (#61) that is the origin serving
 * this SPA, while in local dev or any split deployment with an absolute
 * `VITE_API_BASE_URL` it is the backend's (#1129) — a real, copy-pasteable
 * endpoint either way, where a build/deploy-time constant would not be.
 * Guarded for evaluation outside a DOM (like `getEnv` does), where only the
 * neutral placeholder is available.
 */
const DEFAULT_MCP_ENDPOINT =
  typeof window !== 'undefined'
    ? `${getBackendOrigin()}/mcp/v1/common`
    : 'https://connect.example.com/mcp/v1/common'

/**
 * The single, team-agnostic MCP endpoint advertised in client setup snippets.
 * Defaults to this instance's backend origin (above); self-hosters whose
 * public MCP origin differs from it override it with `VITE_MCP_ENDPOINT`.
 */
export const MCP_ENDPOINT = or(
  getEnv('VITE_MCP_ENDPOINT'),
  DEFAULT_MCP_ENDPOINT
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
