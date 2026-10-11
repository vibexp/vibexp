import { LogoMark } from '@vibexp/design-system/react'

import { cn } from '@/lib/utils'

import { type BrandLogoSource, useBrandLogoSource } from './useBrandLockup'
import { useSiteInfo } from './useSiteInfo'

interface BrandLogoProps {
  /**
   * Height of the logo in pixels. It sizes both forms: the company image (its
   * width follows its aspect ratio) and the VibeXP mark shown instead, so the
   * layout does not jump when there is no logo or the image fails to load.
   * Defaults to the design-system mark's own size.
   */
  size?: number
  /** Extra classes for the company logo image. */
  className?: string
  /**
   * The logo to show, from `useBrandLockup()`. Pass it whenever a name is
   * rendered beside the logo, so the name and the image are decided together.
   * Without it the component chooses the logo itself.
   */
  source?: BrandLogoSource
  /**
   * Hides the fallback VibeXP mark from assistive technology, for when a name
   * printed beside it already says the same (`useBrandLockup().markIsDecorative`).
   * It never affects a company image, which keeps its alt text.
   */
  decorative?: boolean
}

/**
 * The image's name when no company name is configured. An image is only ever
 * a company's logo, so it is never named after the product.
 */
const COMPANY_LOGO_ALT = 'Company logo'

/** The design-system `LogoMark`'s default size, in pixels. */
const DEFAULT_SIZE = 34

/**
 * The instance's logo: the company's when one is configured and loads, the
 * VibeXP mark otherwise. Which of the two is decided by `useBrandLogoSource`
 * (theme, load failure).
 *
 * The image is always an `<img>`, never inlined markup, so an SVG logo cannot
 * run script.
 */
export function BrandLogo({
  size = DEFAULT_SIZE,
  className,
  source,
  decorative = false,
}: Readonly<BrandLogoProps>) {
  const { company_name } = useSiteInfo()
  // Hooks cannot be called conditionally, so the component always has a
  // source of its own and prefers the one it was handed.
  const ownSource = useBrandLogoSource()
  const { src, onError } = source ?? ownSource

  if (src === null) {
    return decorative ? (
      <LogoMark size={size} aria-hidden="true" />
    ) : (
      <LogoMark size={size} />
    )
  }

  return (
    <img
      src={src}
      alt={company_name ?? COMPANY_LOGO_ALT}
      style={{ height: size }}
      className={cn('w-auto max-w-40 object-contain', className)}
      onError={onError}
    />
  )
}
