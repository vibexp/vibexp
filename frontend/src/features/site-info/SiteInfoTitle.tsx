import { Helmet } from 'react-helmet-async'

import { PRODUCT_NAME } from './siteInfo'
import { useSiteInfo } from './useSiteInfo'

/**
 * The browser tab title: "{company name} · VibeXP", or "VibeXP" when no
 * company name is configured.
 *
 * Mounted once at the app root. A page that sets its own title through a
 * deeper `<Helmet>` (the shared-prompt page) wins, which is the library's own
 * precedence. It follows a site info change without a reload.
 */
export function SiteInfoTitle() {
  const { company_name } = useSiteInfo()
  const title =
    company_name === null ? PRODUCT_NAME : `${company_name} · ${PRODUCT_NAME}`
  return (
    <Helmet>
      <title>{title}</title>
    </Helmet>
  )
}
