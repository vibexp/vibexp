import { STORAGE_KEYS } from '@/constants/storageKeys'
import { sessionStore } from '@/utils/storage'

export const GETTING_STARTED_PATH = '/getting-started'

/** The `?step=` query parameter: which step is on screen, 0-based. */
export const STEP_PARAM = 'step'

/** `/getting-started?step=N`, for links that open a given step. */
export function gettingStartedHref(step: number): string {
  return `${GETTING_STARTED_PATH}?${STEP_PARAM}=${String(step)}`
}

export interface AutoOpenInput {
  user: { id: string; onboarding_completed: boolean } | null
  /** The path the user landed on. */
  pathname: string
  /** The user id the flow was already opened for in this browser session. */
  sessionFlag: string | null
  /** A pending invitation token or a stashed `return_to` resumes instead. */
  pendingInvite: boolean
  pendingReturnTo: boolean
}

/**
 * Whether to open the first-run onboarding on its own. Only on the dashboard
 * (`/`): a deep link (a shared memory, a notification) lands on what was
 * clicked. Only once per browser session, so leaving the flow unfinished
 * never traps the user. Never while a login is resuming somewhere else.
 */
export function shouldAutoOpenOnboarding({
  user,
  pathname,
  sessionFlag,
  pendingInvite,
  pendingReturnTo,
}: AutoOpenInput): boolean {
  if (!user || user.onboarding_completed) return false
  if (pathname !== '/') return false
  if (sessionFlag === user.id) return false
  return !pendingInvite && !pendingReturnTo
}

// The session flag's fallback when sessionStorage is unavailable, so the
// flow still opens at most once per page load.
let autoOpenedInMemory: string | null = null

/** Forget that the flow was opened this session. Tests only. */
export function resetAutoOpened() {
  autoOpenedInMemory = null
}

/**
 * Record that the flow was shown to this user in this browser session, so it
 * does not open on its own again before the session ends.
 */
export function rememberAutoOpened(userId: string) {
  autoOpenedInMemory = userId
  sessionStore.set(STORAGE_KEYS.ONBOARDING_AUTO_OPENED, userId)
}

/** The user id the flow already opened for in this session, if any. */
export function readAutoOpened(): string | null {
  return (
    autoOpenedInMemory ?? sessionStore.get(STORAGE_KEYS.ONBOARDING_AUTO_OPENED)
  )
}
