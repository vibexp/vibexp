import { forwardRef } from 'react'
import { Link, type LinkProps } from 'react-router'

import { BrandLogo } from '@/features/site-info/BrandLogo'
import { PRODUCT_NAME } from '@/features/site-info/siteInfo'
import { useBrandLockup } from '@/features/site-info/useBrandLockup'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
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

/** Edge of the logo box, in pixels: the design-system `LogoMark` tile size. */
const LOGO_SIZE = 34

/**
 * Sidebar brand block: the instance's logo plus a wordmark.
 *
 * - Unbranded (no site info), it is the design-system `LogoMark` tile with the
 *   two-line wordmark ("VibeXP" / tagline), mirroring the DS docs sidebar.
 * - With company branding (epic #1352), the logo is the company's (`BrandLogo`)
 *   and the wordmark is the company name on one line. The product tagline is
 *   VibeXP's, so it is not shown under another company's name, and a company
 *   logo shown without a name stands alone. What is paired with what is
 *   decided by `useBrandLockup`, from what is actually on screen.
 * - `showText` forces the wordmark on; otherwise it shows only in the
 *   expanded desktop sidebar (`expanded` + `lg+`) so the icon rail stays at
 *   its collapsed width. In the rail the wordmark is visually hidden but
 *   still read out, which is what names the link there. The logo follows the same rule for its width: in the
 *   rail it is held to the logo box, however wide the image is.
 * - Forwards ref/props to the underlying `Link` so it composes with Radix
 *   `asChild` slots (e.g. `SheetClose`).
 */
export const SidebarBrand = forwardRef<HTMLAnchorElement, SidebarBrandProps>(
  function SidebarBrand(
    { showText = false, expanded = true, className, ...props },
    ref
  ) {
    const { company_name } = useSiteInfo()
    const { logo, name, markIsDecorative } = useBrandLockup()

    let wordmarkDisplay: string
    let logoWidth: string
    if (showText) {
      wordmarkDisplay = 'flex'
      logoWidth = 'max-w-40'
    } else if (expanded) {
      wordmarkDisplay = 'sr-only lg:not-sr-only lg:flex'
      logoWidth = 'max-w-[34px] lg:max-w-40'
    } else {
      wordmarkDisplay = 'sr-only'
      logoWidth = 'max-w-[34px]'
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
        {/* Where the rail takes the wordmark off screen it stays in the
            accessibility tree (`sr-only`, not `hidden`), so the link is named
            by its text in every form and the VibeXP mark beside it can always
            be decorative. */}
        <BrandLogo
          size={LOGO_SIZE}
          className={cn('shrink-0', logoWidth)}
          source={logo}
          decorative={markIsDecorative}
        />
        {company_name !== null && (
          <span
            className={cn('min-w-0 flex-col leading-tight', wordmarkDisplay)}
          >
            <span className="truncate text-sm font-bold tracking-tight">
              {company_name}
            </span>
          </span>
        )}
        {/* No company name and the VibeXP mark on screen: the product's own
            wordmark and tagline, whether or not a company logo is configured
            (it may be dark-only, or have failed to load). */}
        {company_name === null && name !== null && (
          <span className={cn('flex-col leading-tight', wordmarkDisplay)}>
            <span className="text-sm font-bold tracking-tight">
              {PRODUCT_NAME}
            </span>
            <span className="text-muted-foreground text-xs font-normal">
              Your team&apos;s shared brain
            </span>
          </span>
        )}
      </Link>
    )
  }
)
