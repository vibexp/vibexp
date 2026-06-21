import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { sessionStore } from '@/utils/storage'

/**
 * Resume an in-progress invitation accept flow after the user has signed in.
 *
 * Mounted inside `MainApp` (i.e. only after the AuthGate passes). Reads the
 * pending-invitation token stashed by the `AcceptInvitation` page when an
 * unauthenticated visitor was bounced to sign-in. If the token is present and
 * we're not already on `/invitations/accept/:token`, the user is redirected
 * back to that route — at which point the page is now authenticated and the
 * existing flow takes over.
 *
 * Renders nothing.
 */
export function InvitationResumeAfterAuth() {
  const navigate = useNavigate()
  const location = useLocation()
  const ranRef = useRef(false)

  useEffect(() => {
    if (ranRef.current) return
    ranRef.current = true

    const token = sessionStore.get(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
    if (!token) return

    // Already on the accept page? AcceptInvitation owns the flow from here —
    // including read-and-clear via `checkPendingInvitation()`.
    if (location.pathname.startsWith('/invitations/accept/')) return

    void navigate(`/invitations/accept/${encodeURIComponent(token)}`, {
      replace: true,
    })
  }, [navigate, location.pathname])

  return null
}
