import { MoreHorizontal } from 'lucide-react'
import type { ReactNode } from 'react'

import { SegmentedControl } from '@/components/SegmentedControl'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { BodyViewHandle } from './readingChrome'
import type { BodyViewMode, ReadingAction } from './types'

const VIEW_OPTIONS = [
  { value: 'rendered', label: 'Rendered' },
  { value: 'raw', label: 'Raw' },
] as const

/**
 * How the action list splits across the bar and the "More" menu.
 *
 * Pure, so the rule is testable on its own: actions placed on the `toolbar`
 * are labelled buttons, the rest are menu items, and destructive items come
 * last, after a separator. On a phone (`compact`) only the first toolbar
 * action keeps its button — the bar must fit beside the title at 390px — and
 * the others join the top of the menu, where nothing is lost.
 */
export function splitToolbarActions(
  actions: readonly ReadingAction[],
  compact: boolean
): {
  buttons: ReadingAction[]
  menu: ReadingAction[]
  destructive: ReadingAction[]
} {
  const onBar = actions.filter(a => a.placement === 'toolbar')
  const buttons = compact ? onBar.slice(0, 1) : onBar
  const demoted = compact ? onBar.slice(1) : []
  const rest = actions.filter(a => a.placement !== 'toolbar')
  return {
    buttons,
    menu: [...demoted, ...rest.filter(a => a.tone !== 'destructive')],
    destructive: rest.filter(a => a.tone === 'destructive'),
  }
}

interface ReadingToolbarProps {
  actions: readonly ReadingAction[]
  /** The body's Rendered / Raw state, when the page has a body that offers it. */
  bodyView?: BodyViewHandle | null
  /** Phone layout: one button, everything else in the menu. */
  compact?: boolean
  /** Controls the page adds after the menu (focus mode, the Contents button). */
  trailing?: ReactNode
  className?: string
}

/**
 * The reading page's toolbar: the actions a reader reaches for all the time as
 * labelled buttons, the Rendered / Raw switch, and a "More" menu for the rest
 * — with Delete last in it, behind a separator, instead of being the most
 * prominent control on a page meant for reading.
 *
 * It renders whatever actions it is handed and decides nothing about
 * permissions: a page leaves out what the server's `permissions` array does
 * not grant, exactly as it did when these were rail buttons.
 */
export function ReadingToolbar({
  actions,
  bodyView,
  compact = false,
  trailing,
  className,
}: Readonly<ReadingToolbarProps>) {
  const { buttons, menu, destructive } = splitToolbarActions(actions, compact)
  // On a phone the switch has no room on the bar; the menu offers the view the
  // reader is not looking at.
  const viewItem = compact && bodyView ? bodyView : null
  const hasMenu = menu.length > 0 || destructive.length > 0 || viewItem !== null

  if (buttons.length === 0 && !hasMenu && !bodyView && !trailing) return null

  return (
    <div
      role="toolbar"
      aria-label="Document actions"
      data-testid="reading-toolbar"
      className={cn('flex shrink-0 flex-wrap items-center gap-1.5', className)}
    >
      {buttons.map(action => (
        <Button
          key={action.id}
          type="button"
          size="action"
          variant={action.emphasis === 'primary' ? 'default' : 'outline'}
          disabled={action.disabled}
          onClick={action.onClick}
          data-testid={action.testId}
        >
          <action.icon className="size-4" aria-hidden />
          {action.label}
        </Button>
      ))}

      {bodyView && !compact && (
        <SegmentedControl
          size="sm"
          aria-label="Body view"
          options={VIEW_OPTIONS}
          value={bodyView.mode}
          onChange={value => {
            bodyView.setMode(value as BodyViewMode)
          }}
        />
      )}

      {hasMenu && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            {/* A real button, so the menu anchors on a box that exists (#891). */}
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              aria-label="More actions"
              data-testid="reading-more-actions"
              className="size-8"
            >
              <MoreHorizontal className="size-4" aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-44">
            {viewItem && (
              <DropdownMenuItem
                onSelect={() => {
                  viewItem.setMode(viewItem.mode === 'raw' ? 'rendered' : 'raw')
                }}
              >
                {viewItem.mode === 'raw' ? 'View rendered' : 'View raw source'}
              </DropdownMenuItem>
            )}
            {menu.map(action => (
              <DropdownMenuItem
                key={action.id}
                disabled={action.disabled}
                onSelect={action.onClick}
                data-testid={action.testId}
              >
                <action.icon className="size-4" aria-hidden />
                {action.label}
              </DropdownMenuItem>
            ))}
            {destructive.length > 0 && (menu.length > 0 || viewItem) && (
              <DropdownMenuSeparator />
            )}
            {destructive.map(action => (
              <DropdownMenuItem
                key={action.id}
                disabled={action.disabled}
                onSelect={action.onClick}
                data-testid={action.testId}
                className="text-destructive focus:text-destructive"
              >
                <action.icon className="size-4" aria-hidden />
                {action.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      {trailing}
    </div>
  )
}
