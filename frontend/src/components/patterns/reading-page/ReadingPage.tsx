import {
  type ReactNode,
  type TransitionEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'

import { useReadingShell, useShell } from '@/components/layout/ShellContext'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

import { DetailsColumn, DetailsRail } from './DetailsPanel'
import { ReadingActions } from './ReadingActions'
import type { ReadingAction, ReadingSection } from './types'

/**
 * The article's reading measure. Fixed (not "whatever is left") so folding
 * either side column never stretches lines past comfortable length.
 */
const READING_MEASURE = 'max-w-[72ch]'

/**
 * How long to wait for the column's width transition before scrolling to a
 * section anyway (reduced motion, jsdom, or a transition that never fires).
 */
const EXPAND_SCROLL_FALLBACK_MS = 300

export interface ReadingPageProps {
  title: string
  /** Lead paragraph or a row of badges under the title. */
  description?: ReactNode
  /** Document actions — buttons in the column, icons on the rail, chips on phones. */
  actions?: readonly ReadingAction[]
  /** Details sections. Entries whose `content` is null/false are dropped. */
  sections?: readonly ReadingSection[]
  /** The article body. */
  children: ReactNode
  className?: string
}

/**
 * The one reading layout every resource detail page uses (#886).
 *
 * - Registers `reading` mode with the shell (full-bleed content row) and,
 *   when there is anything to show, a details panel (header toggle).
 * - `lg+`: article column with a fixed 72ch measure + a sticky details
 *   column that folds to a 48px icon rail. Rail icons reopen the column and
 *   scroll to their section.
 * - `md–lg`: the details open as a right-side sheet from the header toggle.
 * - `< md`: actions render as chips under the title; the details open as a
 *   bottom sheet.
 *
 * Domain-free: no data fetching, no resource knowledge. Resource pages go
 * through `ResourceReadingPage`, which adds the standard sections.
 */
export function ReadingPage({
  title,
  description,
  actions = [],
  sections = [],
  children,
  className,
}: Readonly<ReadingPageProps>) {
  const visibleSections = sections.filter(
    s => s.content !== null && s.content !== undefined && s.content !== false
  )
  const hasDetails = visibleSections.length > 0 || actions.length > 0

  useReadingShell({ details: hasDetails })

  const {
    isTablet,
    isDesktop,
    detailsOpen,
    setDetailsOpen,
    detailsSheetOpen,
    setDetailsSheetOpen,
  } = useShell()

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
      setDetailsOpen(true)
      setPendingSection(sectionId)
    },
    [setDetailsOpen]
  )

  // Scroll once the column has finished widening; the transitionend handler
  // below does it early, this timer guarantees it happens at all.
  useEffect(() => {
    if (!pendingSection || !detailsOpen) return
    const timer = setTimeout(() => {
      scrollToSection(pendingSection)
      setPendingSection(null)
    }, EXPAND_SCROLL_FALLBACK_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [pendingSection, detailsOpen, scrollToSection])

  const handleTransitionEnd = (event: TransitionEvent<HTMLElement>) => {
    if (event.propertyName !== 'width' || !pendingSection) return
    scrollToSection(pendingSection)
    setPendingSection(null)
  }

  return (
    <>
      <div
        className={cn('min-w-0 flex-1', className)}
        data-testid="reading-page"
      >
        <article
          className={cn(
            'mx-auto w-full px-4 py-6 md:px-8 lg:px-12 lg:py-10',
            READING_MEASURE
          )}
        >
          <header className="mb-8">
            <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
            {description && (
              <div className="text-muted-foreground mt-2 text-sm">
                {description}
              </div>
            )}
            {!isTablet && (
              <ReadingActions
                actions={actions}
                layout="chips"
                className="mt-4"
              />
            )}
          </header>
          {children}
        </article>
      </div>

      {hasDetails && isDesktop && (
        <aside
          ref={asideRef}
          aria-label="Details"
          data-testid="reading-details"
          data-state={detailsOpen ? 'open' : 'collapsed'}
          onTransitionEnd={handleTransitionEnd}
          className={cn(
            'bg-background sticky top-14 h-[calc(100dvh-3.5rem)] shrink-0 overflow-y-auto overflow-x-hidden border-l transition-[width] duration-200',
            detailsOpen ? 'w-80' : 'w-12'
          )}
        >
          {detailsOpen ? (
            <DetailsColumn actions={actions} sections={visibleSections} />
          ) : (
            <DetailsRail
              actions={actions}
              sections={visibleSections}
              onExpandTo={expandTo}
            />
          )}
        </aside>
      )}

      {hasDetails && !isDesktop && (
        <Sheet open={detailsSheetOpen} onOpenChange={setDetailsSheetOpen}>
          <SheetContent
            side={isTablet ? 'right' : 'bottom'}
            className={cn(
              'overflow-y-auto p-0',
              isTablet ? 'w-80 sm:max-w-sm' : 'h-[78dvh] rounded-t-xl'
            )}
            data-testid="reading-details-sheet"
          >
            <SheetHeader className="border-b px-5 py-4 text-left">
              <SheetTitle>Details</SheetTitle>
              <SheetDescription className="sr-only">
                Metadata and activity for {title}
              </SheetDescription>
            </SheetHeader>
            <DetailsColumn
              actions={actions}
              sections={visibleSections}
              showActions={isTablet}
            />
          </SheetContent>
        </Sheet>
      )}
    </>
  )
}
