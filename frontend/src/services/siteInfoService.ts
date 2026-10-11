import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// The instance's public company branding (#1355). Unauthenticated: the
// sign-in page shows it before anyone has signed in.
export type SiteInfoResponse = components['schemas']['SiteInfoResponse']

class SiteInfoService {
  /** The branding in effect right now; every value null when not configured. */
  async getSiteInfo(): Promise<SiteInfoResponse> {
    return unwrap(generatedClient.GET('/api/v1/site-info'))
  }
}

export const siteInfoService = new SiteInfoService()
