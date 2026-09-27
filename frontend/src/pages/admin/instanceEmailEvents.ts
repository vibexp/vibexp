/**
 * Cross-component coordination for instance email settings changes (#1192).
 *
 * The admin shell's warning banner and the dashboard's "Instance email" card
 * each fetch the instance email status on mount. The settings page tells them
 * the status is stale after a save or a removal, so both update without a
 * reload. Same shape as `components/invitations/invitationEvents.ts`.
 */

export const INSTANCE_EMAIL_CHANGED_EVENT = 'vx:instance-email-changed'

/** Notify mounted listeners that the instance email settings changed. */
export function emitInstanceEmailChanged(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(INSTANCE_EMAIL_CHANGED_EVENT))
}

/** Subscribe to instance-email-changed events. Returns an unsubscribe fn. */
export function onInstanceEmailChanged(listener: () => void): () => void {
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
      console.error('instance-email-changed listener failed:', err)
    }
  }
  window.addEventListener(INSTANCE_EMAIL_CHANGED_EVENT, handler)
  return () => {
    window.removeEventListener(INSTANCE_EMAIL_CHANGED_EVENT, handler)
  }
}
