import * as React from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * How a supporting panel (metadata, attachments, comments, relations, a chart
 * widget, …) paints itself.
 *
 * - `card` — the default everywhere: its own bordered, rounded, shadowed box
 *   with a 20px gutter. What a dashboard or an admin page wants.
 * - `flat` — no chrome at all: no border, shadow, radius or background, and no
 *   horizontal inset, because the surface around it already provides both. What
 *   the reading page's details column wants (#890): the column is itself a
 *   bordered, padded surface, so a card inside it is a second border around the
 *   same content.
 *
 * The presentation is decided by the *surface*, through context — never by a
 * prop threaded down through pages — so a new resource type, or any page that
 * drops a widget into the details column, gets the right rendering for free.
 */
export type PanelPresentation = 'card' | 'flat'

const PanelPresentationContext = React.createContext<PanelPresentation>('card')

export function PanelPresentationProvider({
  value,
  children,
}: Readonly<{ value: PanelPresentation; children: React.ReactNode }>) {
  return (
    <PanelPresentationContext.Provider value={value}>
      {children}
    </PanelPresentationContext.Provider>
  )
}

/** The presentation of the surface the calling widget is rendered on. */
export function usePanelPresentation(): PanelPresentation {
  return React.useContext(PanelPresentationContext)
}

/**
 * The horizontal gutter a panel's own rows, body and footers use: the card's
 * 20px inset, or nothing in `flat` mode where the containing surface's padding
 * is the only inset. Widgets that hand-roll padded sub-rows (loading states,
 * empty states, footers) use this instead of a literal `px-5`.
 */
export function usePanelInset(): string {
  return usePanelPresentation() === 'flat' ? 'px-0' : 'px-5'
}

/** The panel root: the card box, or nothing at all in `flat` mode. */
const Panel = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => {
  const flat = usePanelPresentation() === 'flat'
  return (
    <div
      ref={ref}
      className={cn(
        !flat &&
          'bg-card text-card-foreground overflow-hidden rounded-lg border shadow-sm',
        className
      )}
      {...props}
    />
  )
})
Panel.displayName = 'Panel'

/** Header row: icon + title on the left, an optional `PanelAction` on the right. */
const PanelHeader = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => {
  const flat = usePanelPresentation() === 'flat'
  return (
    <div
      ref={ref}
      className={cn(
        'flex items-center justify-between gap-3',
        flat ? 'pb-2.5' : 'px-5 pt-5 pb-4',
        className
      )}
      {...props}
    />
  )
})
PanelHeader.displayName = 'PanelHeader'

/**
 * PanelTitle — the heading for a supporting panel or widget (metadata panels,
 * chart cards, attachment lists). A real, level-settable heading element (`as`
 * defaults to `h3`) so each surface picks the correct heading level for
 * document-outline / screen-reader order.
 *
 * `card`: the app's de-facto panel-title scale (`text-base` / 16px semibold) —
 * the design-system `.type-card-title` role is 20px, which reads as a page-hero
 * scale in this app, so we use the standard-scale equivalent instead.
 * `flat`: 14px semibold, the section-label scale of the details column, which
 * is also what leaves room for the header's compact chip action at 320px.
 */
const PanelTitle = React.forwardRef<
  HTMLElement,
  React.HTMLAttributes<HTMLElement> & { as?: React.ElementType }
>(({ className, as: Component = 'h3', ...props }, ref) => {
  const flat = usePanelPresentation() === 'flat'
  return (
    <Component
      ref={ref}
      className={cn(
        'font-semibold tracking-tight',
        flat ? 'text-sm' : 'text-base',
        className
      )}
      {...props}
    />
  )
})
PanelTitle.displayName = 'PanelTitle'

/**
 * The header's action affordance ("Add file", "Add comment", "Add relation").
 *
 * `card`: the standard small outline button. `flat`: the mock's compact chip —
 * 12px text in a hairline-bordered pill. The chip is what makes the header fit
 * at the 320px column width; the full-size button truncated the title (#890).
 */
const PanelAction = React.forwardRef<
  HTMLButtonElement,
  React.ButtonHTMLAttributes<HTMLButtonElement>
>(({ className, type = 'button', ...props }, ref) => {
  const flat = usePanelPresentation() === 'flat'
  return (
    <Button
      ref={ref}
      type={type}
      variant="outline"
      size={flat ? 'chip' : 'sm'}
      className={cn('shrink-0', className)}
      {...props}
    />
  )
})
PanelAction.displayName = 'PanelAction'

/** The panel's content area, inset by the card gutter (nothing when flat). */
const PanelBody = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => {
  const inset = usePanelInset()
  return <div ref={ref} className={cn(inset, className)} {...props} />
})
PanelBody.displayName = 'PanelBody'

/**
 * One key/value row of a hairline-divided list. The hairlines belong to the
 * list container (`divide-y`), not the row, so a row is only padding + scale:
 * the card's roomier 14px rows, or the mock's 13px rows with no inset.
 */
const PanelRow = React.forwardRef<
  HTMLElement,
  React.HTMLAttributes<HTMLElement> & { as?: React.ElementType }
>(({ className, as: Component = 'div', ...props }, ref) => {
  const flat = usePanelPresentation() === 'flat'
  return (
    <Component
      ref={ref}
      className={cn(
        'flex items-center justify-between gap-4 py-2.5',
        flat ? 'text-[13px]' : 'min-h-12 px-5 text-sm',
        className
      )}
      {...props}
    />
  )
})
PanelRow.displayName = 'PanelRow'

export { Panel, PanelAction, PanelBody, PanelHeader, PanelRow, PanelTitle }
