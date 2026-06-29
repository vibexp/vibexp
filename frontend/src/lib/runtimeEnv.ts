/**
 * Runtime environment access (issue #57).
 *
 * In the single combined image (#61) the Go backend serves `/config.js`, which
 * sets `window.__VIBEXP_ENV__` from its own env vars BEFORE this app's module
 * bundle runs. `getEnv` prefers those runtime values and falls back to the
 * build-time `import.meta.env`, so:
 *   - self-hosters can rebrand / configure at deploy time (env var + restart),
 *     with no rebuild; and
 *   - local dev (Vite dev server, no backend-rendered `config.js`) keeps working
 *     off `import.meta.env`.
 *
 * Only deploy-time-relevant, non-secret values flow through here — see the
 * backend `Config.RuntimeFrontendEnv()` allowlist.
 */

declare global {
  interface Window {
    /** Runtime config injected by the backend's `/config.js` (issue #57). */
    __VIBEXP_ENV__?: Record<string, string | undefined>
  }
}

/** Keys are the `VITE_*` names declared on `ImportMetaEnv` (see vite-env.d.ts). */
type EnvKey = Extract<keyof ImportMetaEnv, string>

/**
 * Returns the configured value for `key`, preferring the backend-injected
 * runtime value over the build-time `import.meta.env`. Returns `undefined` when
 * neither provides a non-empty value, so callers can apply their own default.
 */
export function getEnv(key: EnvKey): string | undefined {
  // Prefer the backend-injected runtime config. Reading through a Map (rather
  // than a computed index) keeps the dynamic key safe and lint-clean.
  const runtime =
    typeof window !== 'undefined' ? window.__VIBEXP_ENV__ : undefined
  if (runtime) {
    const fromRuntime = new Map(Object.entries(runtime)).get(key)
    if (typeof fromRuntime === 'string' && fromRuntime !== '') {
      return fromRuntime
    }
  }

  // Fall back to the build-time values. `import.meta.env` is absent under some
  // test runners, so widen and guard before reading.
  const buildEnv = import.meta.env as Record<string, unknown> | undefined
  if (buildEnv) {
    const fromBuild = new Map(Object.entries(buildEnv)).get(key)
    if (typeof fromBuild === 'string' && fromBuild !== '') {
      return fromBuild
    }
  }

  return undefined
}
