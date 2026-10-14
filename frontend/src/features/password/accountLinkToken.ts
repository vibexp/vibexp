import { useState } from 'react'

/**
 * The token of a one-time account link (epic #1426). The server builds the
 * link with the token in the URL fragment (`/account/setup#token=…`,
 * `/account/reset#token=…`), which a browser never sends to a server.
 *
 * `takeAccountLinkToken` reads it and removes the fragment from the address
 * bar at once, before the page makes any request, so it reaches no history
 * entry, bookmark or `Referer`. Keep the result in memory only: never in a
 * query string, router state or storage. Null when there is none.
 */
export function takeAccountLinkToken(): string | null {
  const hash = globalThis.location.hash.replace(/^#/, '')
  if (!hash) return null
  const token = new URLSearchParams(hash).get('token')
  const { pathname, search } = globalThis.location
  // Keep the router's entry state; only the URL changes.
  globalThis.history.replaceState(
    globalThis.history.state,
    '',
    pathname + search
  )
  return token && token.length > 0 ? token : null
}

/**
 * The account link token, taken once when the component first renders: a
 * re-render or a second effect run does not lose it, since the fragment is
 * gone by then.
 */
export function useAccountLinkToken(): string | null {
  const [token] = useState(takeAccountLinkToken)
  return token
}
