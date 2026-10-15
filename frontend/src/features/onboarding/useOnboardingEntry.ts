import { useAuth } from '@/contexts/useAuth'

import { gettingStartedHref } from './autoOpen'
import { ACTIONABLE_STEP_COUNT } from './content'
import {
  countDone,
  isAllDone,
  resumeStep,
  useOnboardingProgress,
} from './progress'

/**
 * What the ways back into the onboarding show and where they go: the header
 * button, the user menu, the command palette and the home card.
 *
 * - `resumeHref` opens the first step not done yet (the start once all are).
 * - `replayHref` opens the first step.
 */
export function useOnboardingEntry() {
  const { user } = useAuth()
  const [progress, updateProgress] = useOnboardingProgress(user?.id)
  return {
    doneCount: countDone(progress),
    total: ACTIONABLE_STEP_COUNT,
    allDone: isAllDone(progress),
    resumeHref: gettingStartedHref(resumeStep(progress)),
    replayHref: gettingStartedHref(0),
    /** Whether the user hid the home card; replaying does not bring it back. */
    cardDismissed: progress.cardDismissed === true,
    dismissCard: () => {
      updateProgress(p => ({ ...p, cardDismissed: true }))
    },
  }
}
