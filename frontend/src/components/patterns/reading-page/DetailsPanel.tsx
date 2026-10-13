import type { ReactNode } from 'react'

import { PanelPresentationProvider } from '@/components/ui/panel'
import { Separator } from '@/components/ui/separator'
import { TooltipProvider } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import {
  RailButton,
  ReadingActions,
  ReadingActionsRail,
} from './ReadingActions'
import type { ReadingAction, ReadingSection } from './types'

interface DetailsColumnProps {
  actions: readonly ReadingAction[]
  sections: readonly ReadingSection[]
  /** Hide the action grid (phones render actions as chips under the title). */
  showActions?: boolean
  /**
   * Pinned above the sections — the reading page's outline. It stays in view
   * while the sections scroll beneath it, and scrolls within itself once it is
   * taller than about 40% of the viewport, so a long outline can never push
   * the details off the screen.
   */
  lead?: ReactNode
  className?: string
}

/**
 * The open details panel: the action grid, then every section stacked with
 * a `data-section` anchor the rail scrolls to. Rendered in the desktop
 * column and inside the tablet/phone sheet alike.
 *
 * The column is itself a bordered, padded surface, so it declares its sections
 * `flat` (#890): every widget built on the `ui/panel` primitives drops its card
 * border, shadow, radius, background and 20px gutter here, and keeps them
 * everywhere else. The presentation travels by context rather than by a prop,
 * so `ResourceReadingPage` and any resource type added later need no changes.
 */
export function DetailsColumn({
  actions,
  sections,
  showActions = true,
  lead,
  className,
}: Readonly<DetailsColumnProps>) {
  return (
    <PanelPresentationProvider value="flat">
      <div
        // Sections are told apart by spacing and their own labels, not by a
        // rule between each: the column is already one bordered surface.
        className={cn('flex flex-col gap-7 p-5', className)}
        data-testid="details-column"
      >
        {lead && (
          <div className="scrollbar-hover bg-background sticky top-0 z-10 -mx-5 -mt-5 max-h-[40dvh] overflow-y-auto px-5 pt-5 pb-3">
            {lead}
          </div>
        )}
        {showActions && actions.length > 0 && (
          <ReadingActions
            actions={actions}
            layout="grid"
            className="border-b pb-5"
          />
        )}
        {sections.map(section => (
          <section
            key={section.id}
            data-section={section.id}
            aria-label={section.label}
            className="scroll-mt-5"
          >
            {section.content}
          </section>
        ))}
      </div>
    </PanelPresentationProvider>
  )
}

interface DetailsRailProps {
  actions: readonly ReadingAction[]
  sections: readonly ReadingSection[]
  onExpandTo: (sectionId: string) => void
}

/**
 * The folded (48px) details rail: icon-only actions, a hairline, then one
 * icon per section. Clicking a section icon reopens the column and scrolls
 * to that section.
 */
export function DetailsRail({
  actions,
  sections,
  onExpandTo,
}: Readonly<DetailsRailProps>) {
  return (
    <div
      className="flex flex-col items-center gap-1 py-3"
      data-testid="details-rail"
    >
      <ReadingActionsRail actions={actions} />
      {actions.length > 0 && sections.length > 0 && (
        <Separator className="my-1.5 w-6" />
      )}
      <TooltipProvider delayDuration={0}>
        <div className="flex flex-col items-center gap-1">
          {sections.map(section => (
            <RailButton
              key={section.id}
              label={section.label}
              icon={section.icon}
              onClick={() => {
                onExpandTo(section.id)
              }}
              testId={`details-rail-${section.id}`}
            />
          ))}
        </div>
      </TooltipProvider>
    </div>
  )
}
