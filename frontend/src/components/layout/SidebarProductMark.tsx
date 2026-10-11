import { LogoMark } from '@vibexp/design-system/react'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PRODUCT_NAME, PRODUCT_URL } from '@/features/site-info/siteInfo'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import { cn } from '@/lib/utils'

interface SidebarProductMarkProps {
  /** Force the label on at every size (mobile drawer, always expanded). */
  showText?: boolean
  /**
   * Whether the desktop (`lg+`) sidebar is in its expanded state. When false
   * the mark stays alone, in its rail form, at every size. Defaults to true.
   */
  expanded?: boolean
}

/**
 * The VibeXP mark at the foot of every sidebar, linking to vibexp.io.
 *
 * It is always there. An instance can put its own logo and name at the top of
 * the sidebar, but no setting hides the product's mark (epic #1352, decision
 * 5), so this component deliberately takes no prop and reads no value that
 * could turn it off. Site info only changes its wording: "Powered by VibeXP"
 * under a company's branding, plain "VibeXP" otherwise.
 *
 * The label shows wherever `SidebarBrand`'s wordmark does; in the icon rail
 * the mark stands alone and the label moves to a tooltip. The desktop forms
 * need a `TooltipProvider` ancestor, like every other tooltip in the shell;
 * the drawer form (`showText`) renders no tooltip.
 */
export function SidebarProductMark({
  showText = false,
  expanded = true,
}: Readonly<SidebarProductMarkProps>) {
  const { hasCompanyBranding } = useSiteInfo()
  const label = hasCompanyBranding ? `Powered by ${PRODUCT_NAME}` : PRODUCT_NAME

  const link = (labelDisplay: string) => (
    <a
      href={PRODUCT_URL}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={label}
      className={cn(
        'text-muted-foreground hover:text-foreground flex items-center justify-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors',
        showText && 'justify-start',
        !showText && expanded && 'lg:justify-start'
      )}
    >
      <LogoMark size={20} />
      <span className={labelDisplay}>{label}</span>
    </a>
  )

  // In a drawer the label is always visible, so there is nothing for a
  // tooltip to add (and no TooltipProvider to rely on).
  if (showText) {
    return (
      <div className="border-t px-3.5 py-2" data-testid="sidebar-product-mark">
        {link('inline')}
      </div>
    )
  }

  return (
    <div
      className={cn('border-t px-2 py-2', expanded && 'lg:px-3.5')}
      data-testid="sidebar-product-mark"
    >
      <Tooltip>
        {/* The slotted <a> IS the popper's anchor, so it must be a real box
            with a plain string className: no `display: contents` wrapper and
            no function className (#891, see RailLinkWithTooltip). */}
        <TooltipTrigger asChild>
          {link(expanded ? 'hidden lg:inline' : 'hidden')}
        </TooltipTrigger>
        {/* Only where the label is hidden; elsewhere it would repeat it. */}
        <TooltipContent side="right" className={expanded ? 'lg:hidden' : ''}>
          {label}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
