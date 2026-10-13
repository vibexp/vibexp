import { Helmet } from 'react-helmet-async'

import { siteTitleSuffix } from './siteInfo'
import { useSiteInfo } from './useSiteInfo'

/**
 * The browser tab title: "{company name} · VibeXP", or "VibeXP" when no
 * company name is configured.
 *
 * Mounted once at the app root, for the pages outside the app shell; inside
 * it `useDocumentTitle` puts the page's own name in front of the same ending. A page that sets its own title through a
 * deeper `<Helmet>` (the shared-prompt page) wins, which is the library's own
 * precedence. It follows a site info change without a reload.
 */
export function SiteInfoTitle() {
  const { company_name } = useSiteInfo()
  return (
    <Helmet>
      <title>{siteTitleSuffix(company_name)}</title>
    </Helmet>
  )
}
