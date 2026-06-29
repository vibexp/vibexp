import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import { consumeReturnTo } from '@/utils/returnTo'

/**
 * Resume a `return_to` destination after the user has signed in.
 *
 * Mounted inside `MainApp` (i.e. only after the AuthGate passes). The backend
 * redirects a completed provider login to the app root ("/"), so a path stashed
 * before login (e.g. by the OAuth consent page that bounced a signed-out visitor
 * to `/login?return_to=...`) is read-and-cleared here and navigated to. Absent
 * (or unsafe) `return_to` defaults to "/", in which case there is nothing to do.
 *
 * Renders nothing.
 */
export function ReturnToResumeAfterAuth() {
  const navigate = useNavigate()
  const location = useLocation()
  const ranRef = useRef(false)

  useEffect(() => {
    if (ranRef.current) return
    ranRef.current = true

    const returnTo = consumeReturnTo()
    // No stashed path (defaulted to "/") or already there — nothing to resume.
    if (returnTo === '/' || returnTo === location.pathname + location.search) {
      return
    }

    void navigate(returnTo, { replace: true })
  }, [navigate, location.pathname, location.search])

  return null
}
