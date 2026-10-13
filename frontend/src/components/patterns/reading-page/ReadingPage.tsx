import { List, Maximize2, Minimize2 } from 'lucide-react'
import { type ReactNode, useMemo, useRef, useState } from 'react'

import {
  useBreadcrumbLeaf,
  useReadingShell,
  useShell,
} from '@/components/layout/ShellContext'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { DetailsColumn } from './DetailsPanel'
import { ReadingActions } from './ReadingActions'
import { type BodyViewHandle, ReadingChromeContext } from './readingChrome'
import { ReadingOutline } from './ReadingOutline'
import { ReadingRail } from './ReadingRail'
import { ReadingTitleBar } from './ReadingTitleBar'
import type { ReadingAction, ReadingSection } from './types'
import { FOCUS_MODE_KEY, useFocusMode } from './useFocusMode'
import { useReadingOutline } from './useReadingOutline'

/**
 * The article's reading measure. Fixed (not "whatever is left") so folding
 * either side column never stretches lines. The value itself
 * (`--reading-measure`) lives in `styles/index.css` beside the bleed it is
 * computed against, so the column and the wide-block breakout can never
 * disagree about where the column ends.
 */
const READING_MEASURE = 'max-w-(--reading-measure)'

/**
 * How the article is being consumed. `editing` is the same shell — same
 * measure, same gutters, same folding details column — hosting a form rather
 * than a document (#916); it is deliberately a small branch and never a second
 * layout.
 */
export type ReadingPresentation = 'reading' | 'editing'

export interface ReadingPageProps {
  /**
   * The page's name: the `<h1>`, and the accessible name the details sheet
   * describes itself with even when `heading` replaces the visible `<h1>`.
   */
  title: string
  /**
   * Draws the `<h1>` subdued: the title is a stand-in (an untitled memory's
   * body excerpt), not a name someone chose.
   */
  titleSubdued?: boolean
  /**
   * Replaces the `<h1>` — an edit page's inline title input (#1179). The node
   * owns its own heading semantics.
   */
  heading?: ReactNode
  /** Lead paragraph or a row of badges under the title. */
  description?: ReactNode
  /**
   * The breadcrumb's last crumb — what this page is about ("VibeXP / Artifacts
   * / <title>"). Omit while the page has nothing to name (loading, not found).
   */
  breadcrumb?: string
  /**
   * Document actions. A reading page shows them in the toolbar beside the
   * title (see `ReadingAction.placement`); an edit page keeps them as a grid
   * in the rail and chips on narrow screens.
   */
  actions?: readonly ReadingAction[]
  /** Details sections. Entries whose `content` is null/false are dropped. */
  sections?: readonly ReadingSection[]
  /** The article body. */
  children: ReactNode
  /** `editing` hosts a form in the article; defaults to `reading`. */
  presentation?: ReadingPresentation
  className?: string
}

/**
 * The one reading layout every resource detail page uses (#886).
 *
 * - Registers `reading` mode with the shell (which owns the content row) and,
 *   when there is anything to show, a details panel (header toggle).
 * - A reading page's `<header>` is a sticky title bar carrying the toolbar
 *   (Edit, Copy, the Rendered / Raw switch, a "More" menu, focus mode).
 * - Wide (`xl+` reading, `lg+` editing): article column + a sticky details
 *   column that folds to a 48px icon rail. Rail icons reopen the column and
 *   scroll to their section. The reading page's column opens with the body's
 *   outline, pinned above the sections.
 * - The details column is pinned to the right edge of the content area (the
 *   shell's reading row is full width) and the article centers itself with
 *   `mx-auto` in whatever is left beside it — the design's model, and the
 *   reason the article's auto margins are unconditional (#890).
 * - Below that the details are an overlay drawer from the header toggle (a
 *   right-side sheet; a bottom sheet under `md`), closed by default and
 *   leaving the stored column preference untouched. The outline is reachable
 *   from a "Contents" button in the toolbar.
 * - Focus mode hides the navigation and the details and leaves the text.
 * - `presentation="editing"` is the identical layout hosting an edit form
 *   (#916). It changes nothing structural — the whole point is that leaving
 *   edit mode is not a visual jump — and only keeps the details panel
 *   registered while the page is still loading its resource, so the header
 *   toggle and the column do not appear late.
 *
 * Domain-free: no data fetching, no resource knowledge. Resource pages go
 * through `ResourceReadingPage`, which adds the standard sections.
 */
export function ReadingPage({
  title,
  titleSubdued = false,
  heading,
  description,
  breadcrumb,
  actions = [],
  sections = [],
  children,
  presentation = 'reading',
  className,
}: Readonly<ReadingPageProps>) {
  const visibleSections = sections.filter(
    s => s.content !== null && s.content !== undefined && s.content !== false
  )
  // A page handed a `heading` node (the edit page's inline title input) owns
  // its header; everything else gets the title bar and its toolbar.
  const reading = presentation === 'reading' && heading === undefined
  // A reading page's actions are in its toolbar, so only sections make a
  // panel. An edit page paints the shell before its resource resolves, so it
  // has no sections and no Save/Cancel yet; registering the panel anyway keeps
  // the header toggle and the column present across that fetch.
  const hasDetails = reading ? visibleSections.length > 0 : true

  useReadingShell({ details: hasDetails, wideRail: reading })
  useBreadcrumbLeaf(breadcrumb)

  const {
    isTablet,
    isDesktop,
    detailsInline,
    detailsOpen,
    setDetailsOpen,
    detailsSheetOpen,
    setDetailsSheetOpen,
  } = useShell()

  // The body hands its Rendered / Raw state up so the toolbar can draw the
  // switch (see `readingChrome`).
  const [bodyView, setBodyView] = useState<BodyViewHandle | null>(null)
  const chrome = useMemo(() => ({ setBodyView }), [])

  const articleRef = useRef<HTMLElement>(null)
  const outline = useReadingOutline(articleRef, reading)
  const { focusMode, toggleFocusMode } = useFocusMode(reading)
  const [contentsOpen, setContentsOpen] = useState(false)

  const railRendered = hasDetails && detailsInline && !focusMode
  // The outline lives in the column. Where there is no column on screen (a
  // drawer, a phone, focus mode) the toolbar offers it as a sheet; a column
  // the reader folded is their choice, and gets no button.
  const showContentsButton = outline.entries.length > 0 && !railRendered

  // A form's Save is not a convenience: where there is no rail, the chips
  // cover it. A reading page has no chip row — its toolbar is in the header at
  // every width.
  const actionsAsChips = !reading && !isDesktop

  return (
    <>
      <div
        // `@container` makes the column a size container, for descendants that
        // size themselves from the room the folded rail or a wide window frees.
        className={cn('@container min-w-0 flex-1', className)}
        data-testid="reading-page"
        data-presentation={presentation}
        data-focus={focusMode ? 'true' : undefined}
      >
        <article
          ref={articleRef}
          className={cn(
            'reading-article mx-auto w-full px-4 py-6 md:px-8 lg:px-12 lg:py-10',
            READING_MEASURE
          )}
        >
          {reading ? (
            <>
              <ReadingTitleBar
                title={title}
                titleSubdued={titleSubdued}
                actions={actions}
                bodyView={bodyView}
                compact={!isTablet}
                trailing={
                  <ToolbarExtras
                    showContents={showContentsButton}
                    onOpenContents={() => {
                      setContentsOpen(true)
                    }}
                    // Hiding two side panels means nothing where there are
                    // none: focus mode is offered from `lg`, where the
                    // navigation is a column.
                    showFocus={isDesktop}
                    focusMode={focusMode}
                    onToggleFocus={toggleFocusMode}
                  />
                }
              />
              {description && (
                // A lead paragraph is prose: capped at a prose measure even in
                // the wide column, so a long summary does not run 150
                // characters per line at 14px.
                <div className="text-muted-foreground max-w-3xl text-sm">
                  {description}
                </div>
              )}
              <div className="h-8" aria-hidden="true" />
            </>
          ) : (
            <PlainHeader
              title={title}
              heading={heading}
              description={description}
              chips={actionsAsChips ? actions : undefined}
            />
          )}
          <ReadingChromeContext.Provider value={chrome}>
            {children}
          </ReadingChromeContext.Provider>
        </article>
      </div>

      {railRendered && (
        <ReadingRail
          presentation={presentation}
          open={detailsOpen}
          onOpen={() => {
            setDetailsOpen(true)
          }}
          actions={reading ? [] : actions}
          sections={visibleSections}
          lead={
            outline.entries.length > 0 ? (
              <ReadingOutline outline={outline} />
            ) : undefined
          }
        />
      )}

      {hasDetails && !detailsInline && (
        <DetailsSheet
          open={detailsSheetOpen}
          onOpenChange={setDetailsSheetOpen}
          side={isTablet ? 'right' : 'bottom'}
          presentation={presentation}
          title={title}
        >
          <DetailsColumn
            actions={reading ? [] : actions}
            sections={visibleSections}
            // Never twice: the grid exists for the widths the chip row does
            // not cover, and while editing that row reaches up to `lg`.
            showActions={isTablet && !actionsAsChips}
          />
        </DetailsSheet>
      )}

      {reading && (
        <Sheet open={contentsOpen} onOpenChange={setContentsOpen}>
          <SheetContent
            side={isTablet ? 'right' : 'bottom'}
            className={cn(
              'overflow-y-auto p-5',
              isTablet
                ? 'w-details-column sm:max-w-sm'
                : 'max-h-[78dvh] rounded-t-xl'
            )}
            data-testid="reading-contents-sheet"
          >
            <SheetHeader className="sr-only">
              <SheetTitle>Contents</SheetTitle>
              <SheetDescription>{`Sections of ${title}`}</SheetDescription>
            </SheetHeader>
            <ReadingOutline
              outline={outline}
              onNavigate={() => {
                setContentsOpen(false)
              }}
            />
          </SheetContent>
        </Sheet>
      )}
    </>
  )
}

interface ToolbarExtrasProps {
  showContents: boolean
  onOpenContents: () => void
  showFocus: boolean
  focusMode: boolean
  onToggleFocus: () => void
}

/**
 * What the reading page adds to the end of its toolbar: the "Contents" button
 * (wherever the outline is not on screen) and the focus-mode toggle.
 */
function ToolbarExtras({
  showContents,
  onOpenContents,
  showFocus,
  focusMode,
  onToggleFocus,
}: Readonly<ToolbarExtrasProps>) {
  const focusLabel = focusMode ? 'Exit focus mode' : 'Focus mode'
  const FocusIcon = focusMode ? Minimize2 : Maximize2
  return (
    <>
      {showContents && (
        <Button
          type="button"
          size="action"
          variant="outline"
          onClick={onOpenContents}
          data-testid="reading-contents-button"
        >
          <List className="size-4" aria-hidden />
          Contents
        </Button>
      )}
      {showFocus && (
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              {/* A real button: the tooltip anchors on its box (#891). */}
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                className="size-8"
                aria-label={focusLabel}
                aria-pressed={focusMode}
                aria-keyshortcuts={FOCUS_MODE_KEY}
                data-testid="reading-focus-toggle"
                onClick={onToggleFocus}
              >
                <FocusIcon className="size-4" aria-hidden />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {focusLabel}
              <kbd className="bg-muted text-muted-foreground ml-2 rounded px-1 font-mono text-[0.7rem]">
                {focusMode ? 'Esc' : FOCUS_MODE_KEY}
              </kbd>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
    </>
  )
}

/** The header of a page that is not a reading view: the edit page's form. */
function PlainHeader({
  title,
  heading,
  description,
  chips,
}: Readonly<{
  title: string
  heading: ReactNode
  description: ReactNode
  /** Save / Cancel as a chip row, on the widths with no rail to hold them. */
  chips: readonly ReadingAction[] | undefined
}>) {
  return (
    <header className="mb-8">
      {heading ?? (
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      )}
      {description && (
        <div className="text-muted-foreground mt-2 max-w-3xl text-sm">
          {description}
        </div>
      )}
      {chips && (
        <ReadingActions actions={chips} layout="chips" className="mt-4" />
      )}
    </header>
  )
}

/** The details as an overlay drawer, for the widths with no column. */
function DetailsSheet({
  open,
  onOpenChange,
  side,
  presentation,
  title,
  children,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  side: 'right' | 'bottom'
  presentation: ReadingPresentation
  title: string
  children: ReactNode
}>) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={side}
        className={cn(
          'overflow-y-auto p-0',
          side === 'right'
            ? 'w-details-column sm:max-w-sm'
            : 'h-[78dvh] rounded-t-xl'
        )}
        data-testid="reading-details-sheet"
        data-presentation={presentation}
      >
        <SheetHeader className="border-b px-5 py-4 text-left">
          <SheetTitle>Details</SheetTitle>
          <SheetDescription className="sr-only">
            {presentation === 'editing'
              ? `Fields and actions for ${title}`
              : `Metadata and activity for ${title}`}
          </SheetDescription>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  )
}
