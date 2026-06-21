import logoUrl from '@vibexp/design-system/brand/logo.svg'
import { forwardRef } from 'react'
import { Link, type LinkProps } from 'react-router-dom'

import { cn } from '@/lib/utils'

interface SidebarBrandProps extends Omit<LinkProps, 'to'> {
  /** Force the wordmark on (mobile Sheet, always expanded). */
  showText?: boolean
}

/**
 * Sidebar brand block — the released design-system logo tile
 * (`@vibexp/design-system/brand/logo.svg`) plus a two-line wordmark,
 * mirroring the DS docs sidebar ("VibeXP" / subtitle).
 *
 * - `showText` forces the wordmark on; by default it's hidden in the collapsed
 *   icon rail (< lg) so only the tile shows and the rail stays 60px wide.
 * - Forwards ref/props to the underlying `Link` so it composes with Radix
 *   `asChild` slots (e.g. `SheetClose`).
 */
export const SidebarBrand = forwardRef<HTMLAnchorElement, SidebarBrandProps>(
  function SidebarBrand({ showText = false, className, ...props }, ref) {
    return (
      <Link
        ref={ref}
        to="/"
        className={cn(
          // Compact band on the collapsed rail / mobile sheet; on lg+ the logo
          // gets the airier inset of the DS reference (≈24px left, ≈24px top).
          'flex items-center gap-2.5 px-3.5 py-3 transition-opacity hover:opacity-80',
          'lg:px-6 lg:pb-4 lg:pt-6',
          className
        )}
        {...props}
      >
        <img
          src={logoUrl}
          alt="VibeXP"
          width={34}
          height={34}
          className="size-[34px] shrink-0 rounded-[9px]"
        />
        <span
          className={cn(
            'flex-col leading-tight',
            showText ? 'flex' : 'hidden lg:flex'
          )}
        >
          <span className="text-sm font-bold tracking-tight">VibeXP</span>
          <span className="text-muted-foreground text-xs font-normal">
            AI Command Center
          </span>
        </span>
      </Link>
    )
  }
)
