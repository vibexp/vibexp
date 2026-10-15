import { useCallback, useSyncExternalStore } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import { ACTIONABLE_STEP_COUNT, LAST_STEP, STEP_COUNT } from './content'

export type StepStatus = 'todo' | 'done' | 'skipped'

/**
 * How far one user got through the onboarding, in this browser. The server
 * only knows whether the flow is finished (`onboarding_completed`); which
 * steps were done or skipped is a per-browser convenience, so losing it (a
 * private window, cleared site data) only resets the checklist.
 */
export interface OnboardingProgress {
  status: readonly StepStatus[]
  lastStep: number
  /** The home page's "Finish setting up" card was dismissed for good. */
  cardDismissed?: boolean
}

type ProgressByUser = Record<string, OnboardingProgress>

const STATUSES: readonly StepStatus[] = ['todo', 'done', 'skipped']

export const EMPTY_PROGRESS: OnboardingProgress = {
  status: Array.from({ length: STEP_COUNT }, () => 'todo' as const),
  lastStep: 0,
}

export function clampStep(step: number): number {
  if (!Number.isFinite(step)) return 0
  return Math.max(0, Math.min(LAST_STEP, Math.trunc(step)))
}

/** Whatever storage held, as progress; anything malformed reads as none. */
function parseProgress(raw: unknown): OnboardingProgress {
  if (typeof raw !== 'object' || raw === null) return EMPTY_PROGRESS
  const { status, lastStep, cardDismissed } = raw as {
    status?: unknown
    lastStep?: unknown
    cardDismissed?: unknown
  }
  if (!Array.isArray(status)) return EMPTY_PROGRESS
  const stored: readonly unknown[] = status
  return {
    status: EMPTY_PROGRESS.status.map((_, i) => {
      const value = stored.at(i)
      return STATUSES.find(s => s === value) ?? 'todo'
    }),
    lastStep: typeof lastStep === 'number' ? clampStep(lastStep) : 0,
    cardDismissed: cardDismissed === true,
  }
}

// Read once per user, then served from memory: it keeps the snapshot stable
// for useSyncExternalStore, and keeps the flow working for the rest of the
// visit when storage is unavailable.
const cache = new Map<string, OnboardingProgress>()
const listeners = new Set<() => void>()

function readAll(): ProgressByUser {
  const all = storage.getJSON<unknown>(STORAGE_KEYS.ONBOARDING_PROGRESS)
  return typeof all === 'object' && all !== null ? (all as ProgressByUser) : {}
}

export function readProgress(userId: string): OnboardingProgress {
  const cached = cache.get(userId)
  if (cached) return cached
  const progress = parseProgress(readAll()[userId])
  cache.set(userId, progress)
  return progress
}

export function writeProgress(userId: string, progress: OnboardingProgress) {
  cache.set(userId, progress)
  // `storage` already swallows (and logs) a failing write.
  storage.set(STORAGE_KEYS.ONBOARDING_PROGRESS, {
    ...readAll(),
    [userId]: progress,
  })
  listeners.forEach(listener => {
    listener()
  })
}

/** Forget the in-memory copy. Tests only. */
export function resetProgressCache() {
  cache.clear()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** Sets one step's status. A done step stays done when it is later skipped. */
export function withStepStatus(
  progress: OnboardingProgress,
  step: number,
  value: StepStatus
): OnboardingProgress {
  const status = progress.status.slice()
  if (status[step] !== 'done' || value === 'done') status[step] = value
  return { ...progress, status }
}

/** Done steps, out of the five that can be done (the celebration is not one). */
export function countDone(progress: OnboardingProgress): number {
  return progress.status
    .slice(0, ACTIONABLE_STEP_COUNT)
    .filter(s => s === 'done').length
}

export function isAllDone(progress: OnboardingProgress): boolean {
  return countDone(progress) === ACTIONABLE_STEP_COUNT
}

/** Where "Resume" lands: the first step not done yet, or the start. */
export function resumeStep(progress: OnboardingProgress): number {
  const i = progress.status
    .slice(0, ACTIONABLE_STEP_COUNT)
    .findIndex(s => s !== 'done')
  return i < 0 ? 0 : i
}

/**
 * One user's onboarding progress, shared live between everything on screen
 * that shows it (the flow, the profile menu, the command palette, the home card).
 */
export function useOnboardingProgress(userId: string | undefined) {
  const progress = useSyncExternalStore(subscribe, () =>
    userId ? readProgress(userId) : EMPTY_PROGRESS
  )
  const update = useCallback(
    (change: (current: OnboardingProgress) => OnboardingProgress) => {
      if (!userId) return
      writeProgress(userId, change(readProgress(userId)))
    },
    [userId]
  )
  return [progress, update] as const
}
