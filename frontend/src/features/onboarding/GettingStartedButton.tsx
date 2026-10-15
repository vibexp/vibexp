import { Icon } from '@vibexp/design-system/react'
import { Link } from 'react-router'

import { useOnboardingEntry } from './useOnboardingEntry'

/**
 * The app header's way back into the onboarding, with how many of its five
 * steps are done. Opens it where the user left off. On a phone it keeps the
 * icon and the count; the label is still its accessible name.
 */
export function GettingStartedButton() {
  const { doneCount, total, resumeHref } = useOnboardingEntry()

  return (
    <Link
      to={resumeHref}
      aria-label={`Getting started, ${String(doneCount)} of ${String(total)} steps done`}
      title="Getting started"
      data-testid="getting-started-button"
      className="bg-background text-foreground hover:bg-accent inline-flex h-[34px] shrink-0 items-center gap-2 rounded-md border px-2.5 text-sm font-medium whitespace-nowrap sm:px-3"
    >
      <Icon name="rocket" size={15} />
      <span className="hidden sm:inline">Getting started</span>
      <span className="text-muted-foreground text-xs tabular-nums">
        {doneCount}/{total}
      </span>
    </Link>
  )
}
