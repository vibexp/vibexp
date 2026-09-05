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
  className?: string
}

/**
 * The open details panel: the action grid, then every section stacked with
 * a `data-section` anchor the rail scrolls to. Rendered in the desktop
 * column and inside the tablet/phone sheet alike.
 */
export function DetailsColumn({
  actions,
  sections,
  showActions = true,
  className,
}: Readonly<DetailsColumnProps>) {
  return (
    <div
      className={cn('flex flex-col gap-4 p-4', className)}
      data-testid="details-column"
    >
      {showActions && actions.length > 0 && (
        <ReadingActions
          actions={actions}
          layout="grid"
          className="border-b pb-4"
        />
      )}
      {sections.map(section => (
        <section
          key={section.id}
          data-section={section.id}
          aria-label={section.label}
          className="scroll-mt-4"
        >
          {section.content}
        </section>
      ))}
    </div>
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
