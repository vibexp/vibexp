import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useAuth } from '@/contexts/useAuth'
import { DEFAULT_RETURN_TO, sanitizeReturnTo } from '@/utils/returnTo'
import { sessionStore } from '@/utils/storage'

import {
  GETTING_STARTED_PATH,
  readAutoOpened,
  rememberAutoOpened,
  shouldAutoOpenOnboarding,
} from './autoOpen'

/**
 * Opens the first-run onboarding for a user who has not finished it (see
 * `shouldAutoOpenOnboarding`). Mounted in `MainApp` after the auth gate and
 * before the resume components, so its effect sees a pending invitation or
 * `return_to` before they consume it. Finishing or skipping the flow sets
 * `onboarding_completed`, and it never opens on its own again.
 *
 * Renders nothing.
 */
export function OnboardingRedirect() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const ranRef = useRef(false)

  useEffect(() => {
    if (ranRef.current || !user) return
    ranRef.current = true

    const open = shouldAutoOpenOnboarding({
      user,
      pathname,
      sessionFlag: readAutoOpened(),
      pendingInvite: sessionStore.has(STORAGE_KEYS.PENDING_INVITATION_TOKEN),
      pendingReturnTo:
        sanitizeReturnTo(sessionStore.get(STORAGE_KEYS.RETURN_TO)) !==
        DEFAULT_RETURN_TO,
    })
    if (!open) return

    rememberAutoOpened(user.id)
    void navigate(GETTING_STARTED_PATH, { replace: true })
  }, [user, pathname, navigate])

  return null
}
