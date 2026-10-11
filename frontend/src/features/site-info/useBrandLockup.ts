import { useState } from 'react'

import { useResolvedTheme } from '@/lib/theme'

import { PRODUCT_NAME } from './siteInfo'
import { useSiteInfo } from './useSiteInfo'

/**
 * Which logo is on screen: a company image, or (when `src` is null) the VibeXP
 * mark that stands in for it.
 */
export interface BrandLogoSource {
  /** The company logo URL to show, or null when the VibeXP mark is shown. */
  src: string | null
  /** Call when the image fails to load; the VibeXP mark then takes its place. */
  onError: () => void
}

/** A logo and the name that goes beside it. */
export interface BrandLockup {
  logo: BrandLogoSource
  /**
   * The name to print beside the logo, or null for none:
   *
   * - the company name when one is configured;
   * - otherwise "VibeXP" when the VibeXP mark is what is on screen;
   * - otherwise null: a company's own image, with no name configured, stands
   *   alone, and "VibeXP" is never printed beside another company's logo.
   */
  name: string | null
  /**
   * Whether the VibeXP mark is shown with a name beside it. The name already
   * says what the mark would, so the mark is hidden from assistive technology
   * and a screen reader does not hear "VibeXP, VibeXP".
   */
  markIsDecorative: boolean
}

/**
 * Chooses the logo to show and remembers a load failure.
 *
 * - In dark mode the dark logo is used when set; the main logo otherwise.
 * - If the image fails to load (a wrong URL, a host that is down) the VibeXP
 *   mark takes its place rather than a broken-image icon. The fallback is per
 *   URL: a new logo URL gets a fresh attempt.
 */
export function useBrandLogoSource(): BrandLogoSource {
  const { logo_url, logo_dark_url } = useSiteInfo()
  const resolvedTheme = useResolvedTheme()
  const [failedSrc, setFailedSrc] = useState<string | null>(null)

  const chosen =
    resolvedTheme === 'dark' && logo_dark_url !== null
      ? logo_dark_url
      : logo_url

  return {
    src: chosen !== null && chosen !== failedSrc ? chosen : null,
    onError: () => {
      setFailedSrc(chosen)
    },
  }
}

/**
 * The instance's lockup: the logo to show and the name to print beside it.
 *
 * This is the one place the pairing rule lives. It is decided by what is
 * actually displayed, not by what is configured: a company logo that is set
 * but not on screen (a dark-only logo in light theme, or an image that failed
 * to load) leaves the VibeXP mark showing, and that mark gets its name.
 *
 * Pass `logo` to `BrandLogo` as its `source`, so the component and the name
 * agree about the same image.
 */
export function useBrandLockup(): BrandLockup {
  const { company_name } = useSiteInfo()
  const logo = useBrandLogoSource()

  let name: string | null = company_name
  if (name === null && logo.src === null) {
    name = PRODUCT_NAME
  }

  return { logo, name, markIsDecorative: logo.src === null && name !== null }
}
