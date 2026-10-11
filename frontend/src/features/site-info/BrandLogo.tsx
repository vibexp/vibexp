import { LogoMark } from '@vibexp/design-system/react'
import { useState } from 'react'

import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

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
}

/** The design-system `LogoMark`'s default size, in pixels. */
const DEFAULT_SIZE = 34

/**
 * The instance's logo: the company's when one is configured, the VibeXP mark
 * otherwise.
 *
 * - In dark mode the dark logo is used when set; the main logo otherwise.
 * - The image is always an `<img>`, never inlined markup, so an SVG logo
 *   cannot run script.
 * - If the image fails to load (a wrong URL, a host that is down) the VibeXP
 *   mark takes its place rather than a broken-image icon. The fallback is per
 *   URL: a new logo URL gets a fresh attempt.
 */
export function BrandLogo({
  size = DEFAULT_SIZE,
  className,
}: Readonly<BrandLogoProps>) {
  const { logo_url, logo_dark_url, displayName } = useSiteInfo()
  const { resolvedTheme } = useTheme()
  const [failedSrc, setFailedSrc] = useState<string | null>(null)

  const src =
    resolvedTheme === 'dark' && logo_dark_url !== null
      ? logo_dark_url
      : logo_url

  if (src === null || src === failedSrc) {
    return <LogoMark size={size} />
  }

  return (
    <img
      src={src}
      alt={displayName}
      style={{ height: size }}
      className={cn('w-auto max-w-40 object-contain', className)}
      onError={() => {
        setFailedSrc(src)
      }}
    />
  )
}
