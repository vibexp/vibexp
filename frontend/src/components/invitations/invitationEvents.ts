/**
 * Cross-component coordination for invitation state changes.
 *
 * The dashboard banner is mounted once at the layout level and only fetches
 * pending invitations on its own mount. Other surfaces (the Teams page,
 * `useAcceptAndEnterTeam`, the AcceptInvitation post-auth handshake) need a
 * way to tell the banner "your data is stale" without owning a shared store.
 *
 * A plain `window` CustomEvent is enough: it's synchronous, cross-component,
 * SSR-safe (we guard `window`), and stays out of the React tree.
 */

export const INVITATIONS_CHANGED_EVENT = 'vx:invitations-changed'

/**
 * Notify any mounted listeners that the current user's pending invitations
 * have changed (accepted, rejected, dismissed-banner reset, etc.).
 */
export function emitInvitationsChanged(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(INVITATIONS_CHANGED_EVENT))
}

/**
 * Subscribe to invitation-state-changed events. Returns an unsubscribe fn.
 */
export function onInvitationsChanged(listener: () => void): () => void {
  const noop = () => {
    /* server-side: no-op */
  }
  if (typeof window === 'undefined') return noop
  const handler = () => {
    // Isolate listener failures so one bad listener can't stop the others
    // from running on a synchronous CustomEvent fan-out.
    try {
      listener()
    } catch (err) {
      console.error('invitations-changed listener failed:', err)
    }
  }
  window.addEventListener(INVITATIONS_CHANGED_EVENT, handler)
  return () => {
    window.removeEventListener(INVITATIONS_CHANGED_EVENT, handler)
  }
}
