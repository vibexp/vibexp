import { LogoMark } from '@vibexp/design-system/react'

import { cn } from '@/lib/utils'

import { PRODUCT_NAME, PRODUCT_URL } from './siteInfo'

interface PoweredByProps {
  /** Extra classes for the link. */
  className?: string
}

/**
 * The "Powered by VibeXP" credit shown on a company-branded instance.
 *
 * It renders unconditionally and reads no site info, so no configured value
 * can change or remove it: the caller decides when to show it, by
 * `hasCompanyBranding`.
 */
export function PoweredBy({ className }: Readonly<PoweredByProps>) {
  return (
    <a
      href={PRODUCT_URL}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(
        'text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-xs transition-colors',
        className
      )}
    >
      <LogoMark size={16} aria-hidden="true" />
      <span>Powered by {PRODUCT_NAME}</span>
    </a>
  )
}
