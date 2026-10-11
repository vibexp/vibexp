import { LogoMark } from '@vibexp/design-system/react'
import { useState } from 'react'

import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

import { useSiteInfo } from './useSiteInfo'

interface BrandLogoProps {
  /** Classes for the company logo image (size it here, e.g. `h-9 w-auto`). */
  className?: string
}

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
export function BrandLogo({ className }: Readonly<BrandLogoProps>) {
  const { logo_url, logo_dark_url, displayName } = useSiteInfo()
  const { resolvedTheme } = useTheme()
  const [failedSrc, setFailedSrc] = useState<string | null>(null)

  const src =
    resolvedTheme === 'dark' && logo_dark_url !== null
      ? logo_dark_url
      : logo_url

  if (src === null || src === failedSrc) {
    return <LogoMark />
  }

  return (
    <img
      src={src}
      alt={displayName}
      className={cn('h-9 w-auto max-w-40 object-contain', className)}
      onError={() => {
        setFailedSrc(src)
      }}
    />
  )
}
