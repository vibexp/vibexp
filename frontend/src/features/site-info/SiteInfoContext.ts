import { createContext } from 'react'

import type { DerivedSiteInfo } from './siteInfo'

/**
 * The instance's site info with the shared fallbacks applied. `undefined`
 * outside a `SiteInfoProvider`; read it through `useSiteInfo`.
 */
export const SiteInfoContext = createContext<DerivedSiteInfo | undefined>(
  undefined
)
