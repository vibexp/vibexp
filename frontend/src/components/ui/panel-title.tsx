import * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * PanelTitle — the heading for a supporting panel or widget (artifact metadata
 * panels, chart cards, attachment lists). A real, level-settable heading
 * element (`as` defaults to `h3`) so each surface picks the correct heading
 * level for document-outline / screen-reader order.
 *
 * Sized on the app's de-facto panel-title scale (`text-base` / 16px semibold):
 * the design-system `.type-card-title` role is 20px, which reads as a page-hero
 * scale in this app, so we use the standard-scale equivalent instead.
 */
const PanelTitle = React.forwardRef<
  HTMLElement,
  React.HTMLAttributes<HTMLElement> & { as?: React.ElementType }
>(({ className, as: Component = 'h3', ...props }, ref) => (
  <Component
    ref={ref}
    className={cn('text-base font-semibold tracking-tight', className)}
    {...props}
  />
))
PanelTitle.displayName = 'PanelTitle'

export { PanelTitle }
