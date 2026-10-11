import { useContext } from 'react'

import {
  type DerivedSiteInfo,
  deriveSiteInfo,
  EMPTY_SITE_INFO,
} from './siteInfo'
import { SiteInfoContext } from './SiteInfoContext'

const NOTHING_CONFIGURED: DerivedSiteInfo = deriveSiteInfo(EMPTY_SITE_INFO)

/**
 * The instance's site info with the shared fallbacks applied.
 *
 * Unlike `useAuth`, this does not throw outside its provider: it answers
 * "nothing configured", the same thing an unbranded instance shows. Branding
 * is decoration, so a component rendered on its own (a test, a storybook-style
 * harness) still renders, as VibeXP.
 */
export function useSiteInfo(): DerivedSiteInfo {
  return useContext(SiteInfoContext) ?? NOTHING_CONFIGURED
}
