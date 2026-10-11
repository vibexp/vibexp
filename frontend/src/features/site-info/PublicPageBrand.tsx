import { cn } from '@/lib/utils'

import { BrandLogo } from './BrandLogo'
import { PoweredBy } from './PoweredBy'
import { useBrandLockup } from './useBrandLockup'
import { useSiteInfo } from './useSiteInfo'

/**
 * The company lockup for the standalone public pages (invitation, consent,
 * shared prompt): its logo and, when set, its name. With a website configured
 * the lockup links to it.
 *
 * It renders nothing on an unbranded instance. Those pages show no brand of
 * their own today, and an instance that has configured nothing must keep
 * looking exactly as it does (epic #1352).
 */
export function PublicPageBrand({
  className,
}: Readonly<{ className?: string }>) {
  const { hasCompanyBranding, website } = useSiteInfo()
  const { logo, name, markIsDecorative } = useBrandLockup()
  if (!hasCompanyBranding) return null

  const lockup = (
    <>
      <BrandLogo size={40} source={logo} decorative={markIsDecorative} />
      {name !== null && (
        <span className="text-lg font-semibold tracking-tight">{name}</span>
      )}
    </>
  )
  const classes = cn('inline-flex items-center gap-3', className)

  if (website === null) {
    return (
      <div className={classes} data-testid="public-page-brand">
        {lockup}
      </div>
    )
  }
  return (
    <a
      href={website}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(classes, 'transition-opacity hover:opacity-80')}
      data-testid="public-page-brand"
    >
      {lockup}
    </a>
  )
}

/**
 * The full-height, centred frame of a standalone card page.
 *
 * Unbranded, it is exactly the wrapper those pages always had: the centred
 * container around the card, and nothing else. With company branding, the card
 * gets the company lockup above it and the "Powered by VibeXP" credit below.
 */
export function PublicCardPage({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const { hasCompanyBranding } = useSiteInfo()
  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      {hasCompanyBranding ? (
        <div className="flex w-full flex-col items-center gap-5">
          <PublicPageBrand />
          {children}
          <PoweredBy />
        </div>
      ) : (
        children
      )}
    </div>
  )
}
