import { LogoMark } from '@vibexp/design-system/react'
import { forwardRef } from 'react'
import { Link, type LinkProps } from 'react-router'

import { cn } from '@/lib/utils'

interface SidebarBrandProps extends Omit<LinkProps, 'to'> {
  /** Force the wordmark on at every size (mobile drawer, always expanded). */
  showText?: boolean
  /**
   * Whether the desktop (`lg+`) sidebar is in its expanded state. When false
   * the brand stays in its compact rail form at every size. Defaults to true.
   */
  expanded?: boolean
}

/**
 * Sidebar brand block — the design-system `LogoMark` tile
 * (`@vibexp/design-system/react`, 34px by default) plus a two-line wordmark,
 * mirroring the DS docs sidebar ("VibeXP" / subtitle).
 *
 * - `showText` forces the wordmark on; otherwise it shows only in the
 *   expanded desktop sidebar (`expanded` + `lg+`) so the icon rail stays at
 *   its collapsed width.
 * - Forwards ref/props to the underlying `Link` so it composes with Radix
 *   `asChild` slots (e.g. `SheetClose`).
 */
export const SidebarBrand = forwardRef<HTMLAnchorElement, SidebarBrandProps>(
  function SidebarBrand(
    { showText = false, expanded = true, className, ...props },
    ref
  ) {
    let wordmarkDisplay: string
    if (showText) {
      wordmarkDisplay = 'flex'
    } else if (expanded) {
      wordmarkDisplay = 'hidden lg:flex'
    } else {
      wordmarkDisplay = 'hidden'
    }

    return (
      <Link
        ref={ref}
        to="/"
        className={cn(
          // Compact band on the collapsed rail / mobile sheet; on lg+ the logo
          // gets the airier inset of the DS reference (≈24px left, ≈24px top).
          'flex items-center gap-2.5 px-3.5 py-3 transition-opacity hover:opacity-80',
          expanded && 'lg:px-6 lg:pb-4 lg:pt-6',
          className
        )}
        {...props}
      >
        <LogoMark />
        <span className={cn('flex-col leading-tight', wordmarkDisplay)}>
          <span className="text-sm font-bold tracking-tight">VibeXP</span>
          <span className="text-muted-foreground text-xs font-normal">
            Your team&apos;s shared brain
          </span>
        </span>
      </Link>
    )
  }
)
