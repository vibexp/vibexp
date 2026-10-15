import { Icon } from '@vibexp/design-system/react'
import { Link } from 'react-router'

import { useOnboardingEntry } from './useOnboardingEntry'

/**
 * The home page's "Finish setting up VibeXP" card, from the design's shell
 * view: progress through the onboarding, with Resume and Replay. It shows
 * only while steps are left, and the user can dismiss it for good (most who
 * skip the setup would otherwise carry it forever). Either way the header's
 * "Getting started" stays the way back.
 */
export function OnboardingCard() {
  const {
    doneCount,
    total,
    allDone,
    resumeHref,
    replayHref,
    cardDismissed,
    dismissCard,
  } = useOnboardingEntry()

  if (allDone || cardDismissed) return null

  const pct = Math.round((doneCount / total) * 100)

  return (
    <section aria-labelledby="onboarding-card-title">
      <div className="bg-card relative flex flex-wrap items-center gap-6 rounded-xl border p-6">
        <button
          type="button"
          onClick={dismissCard}
          aria-label="Dismiss"
          title="Dismiss"
          className="text-muted-foreground hover:bg-accent hover:text-foreground absolute top-2 right-2 grid size-8 cursor-pointer place-items-center rounded-md border-none bg-transparent"
        >
          <Icon name="x" size={15} />
        </button>
        <div className="flex flex-[1_1_320px] flex-col gap-1.5">
          <h2
            id="onboarding-card-title"
            className="m-0 text-lg font-semibold tracking-[-0.018em]"
          >
            Finish setting up VibeXP
          </h2>
          <p className="text-muted-foreground m-0 text-sm">
            {doneCount} of {total} steps complete. Pick up where you left off.
          </p>
          <div
            role="progressbar"
            aria-label="Setup progress"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
            className="bg-muted mt-2.5 h-1 max-w-[360px] overflow-hidden rounded"
          >
            <div
              className="bg-primary h-full transition-[width] duration-300"
              style={{ width: `${String(pct)}%` }}
            />
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link
            to={resumeHref}
            className="bg-primary text-primary-foreground inline-flex h-10 items-center gap-2 rounded-md px-4 text-sm font-medium hover:opacity-[0.88]"
          >
            Resume setup
            <Icon name="arrowRight" size={15} />
          </Link>
          <Link
            to={replayHref}
            className="bg-background text-foreground hover:bg-accent inline-flex h-10 items-center gap-2 rounded-md border px-4 text-sm font-medium"
          >
            <Icon name="play" size={14} />
            Replay onboarding
          </Link>
        </div>
      </div>
      <p className="text-muted-foreground mx-1 mt-4 mb-0 text-[13px]">
        Onboarding is always available from{' '}
        <strong className="text-foreground font-medium">Getting started</strong>{' '}
        in the top bar.
      </p>
    </section>
  )
}
