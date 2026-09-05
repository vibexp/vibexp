import { Button, type ButtonProps } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import type { ReadingAction } from './types'

function buttonVariant(action: ReadingAction): ButtonProps['variant'] {
  if (action.tone === 'destructive') return 'destructive'
  if (action.emphasis === 'primary') return 'default'
  return 'outline'
}

interface ReadingActionsProps {
  actions: readonly ReadingAction[]
  /**
   * - `grid`: two-column button grid at the top of the details column.
   * - `chips`: wrapping row of small buttons under the title (phones).
   */
  layout: 'grid' | 'chips'
  className?: string
}

/** Full (icon + label) rendering of the action list. */
export function ReadingActions({
  actions,
  layout,
  className,
}: Readonly<ReadingActionsProps>) {
  if (actions.length === 0) return null

  return (
    <div
      className={cn(
        layout === 'grid'
          ? 'grid grid-cols-2 gap-1.5'
          : 'flex flex-wrap gap-1.5',
        className
      )}
      data-testid={`reading-actions-${layout}`}
    >
      {actions.map(action => (
        <Button
          key={action.id}
          type="button"
          size="sm"
          variant={buttonVariant(action)}
          disabled={action.disabled}
          onClick={action.onClick}
          data-testid={action.testId}
          className={layout === 'grid' ? 'w-full' : undefined}
        >
          <action.icon className="size-4" aria-hidden />
          {action.label}
        </Button>
      ))}
    </div>
  )
}

interface RailButtonProps {
  label: string
  icon: ReadingAction['icon']
  onClick: () => void
  disabled?: boolean
  destructive?: boolean
  testId?: string
}

/** One icon-only button on the folded rail, labelled by a tooltip. */
export function RailButton({
  label,
  icon: Icon,
  onClick,
  disabled,
  destructive,
  testId,
}: Readonly<RailButtonProps>) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          disabled={disabled}
          onClick={onClick}
          data-testid={testId}
          className={cn(
            'text-muted-foreground',
            destructive && 'text-destructive hover:text-destructive'
          )}
        >
          <Icon className="size-4" aria-hidden />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">{label}</TooltipContent>
    </Tooltip>
  )
}

interface ReadingActionsRailProps {
  actions: readonly ReadingAction[]
}

/** Icon-only rendering of the action list for the folded rail. */
export function ReadingActionsRail({
  actions,
}: Readonly<ReadingActionsRailProps>) {
  if (actions.length === 0) return null
  return (
    <TooltipProvider delayDuration={0}>
      <div className="flex flex-col items-center gap-1">
        {actions.map(action => (
          <RailButton
            key={action.id}
            label={action.label}
            icon={action.icon}
            onClick={action.onClick}
            disabled={action.disabled}
            destructive={action.tone === 'destructive'}
            testId={action.testId}
          />
        ))}
      </div>
    </TooltipProvider>
  )
}
