import {
  type ReactNode,
  type TransitionEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'

import { cn } from '@/lib/utils'

import { DetailsColumn, DetailsRail } from './DetailsPanel'
import type { ReadingPresentation } from './ReadingPage'
import type { ReadingAction, ReadingSection } from './types'

/**
 * How long to wait for the column's width transition before scrolling to a
 * section anyway (reduced motion, jsdom, or a transition that never fires).
 */
const EXPAND_SCROLL_FALLBACK_MS = 300

interface ReadingRailProps {
  presentation: ReadingPresentation
  open: boolean
  onOpen: () => void
  /** The edit page's Save / Cancel; a reading page passes none. */
  actions: readonly ReadingAction[]
  sections: readonly ReadingSection[]
  /** Pinned at the top of the open column (the reading page's outline). */
  lead?: ReactNode
}

/**
 * The details column beside the article: sticky under the app header, with
 * its own scroll, folding to a 48px icon rail. A rail icon reopens the column
 * and scrolls to its section once the column has finished widening.
 */
export function ReadingRail({
  presentation,
  open,
  onOpen,
  actions,
  sections,
  lead,
}: Readonly<ReadingRailProps>) {
  const asideRef = useRef<HTMLElement>(null)
  const [pendingSection, setPendingSection] = useState<string | null>(null)

  const scrollToSection = useCallback((sectionId: string) => {
    const target = asideRef.current?.querySelector<HTMLElement>(
      `[data-section="${sectionId}"]`
    )
    if (target && typeof target.scrollIntoView === 'function') {
      target.scrollIntoView({ block: 'start', behavior: 'smooth' })
    }
  }, [])

  const expandTo = useCallback(
    (sectionId: string) => {
      onOpen()
      setPendingSection(sectionId)
    },
    [onOpen]
  )

  // Scroll once the column has finished widening; the transitionend handler
  // below does it early, this timer guarantees it happens at all.
  useEffect(() => {
    if (!pendingSection || !open) return
    const timer = setTimeout(() => {
      scrollToSection(pendingSection)
      setPendingSection(null)
    }, EXPAND_SCROLL_FALLBACK_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [pendingSection, open, scrollToSection])

  const handleTransitionEnd = (event: TransitionEvent<HTMLElement>) => {
    if (event.propertyName !== 'width' || !pendingSection) return
    scrollToSection(pendingSection)
    setPendingSection(null)
  }

  return (
    <aside
      ref={asideRef}
      aria-label="Details"
      data-testid="reading-details"
      data-presentation={presentation}
      data-state={open ? 'open' : 'collapsed'}
      onTransitionEnd={handleTransitionEnd}
      className={cn(
        'scrollbar-hover bg-background sticky top-14 h-[calc(100dvh-3.5rem)] shrink-0 overflow-y-auto overflow-x-hidden border-l transition-[width] duration-200',
        open ? 'w-details-column' : 'w-12'
      )}
    >
      {open ? (
        <DetailsColumn actions={actions} sections={sections} lead={lead} />
      ) : (
        <DetailsRail
          actions={actions}
          sections={sections}
          onExpandTo={expandTo}
        />
      )}
    </aside>
  )
}
