import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the support domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type SupportRequest = components['schemas']['SupportRequest']
export type SupportResponse = components['schemas']['SupportResponse']

class SupportService {
  async submitSupportRequest(data: SupportRequest): Promise<SupportResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/support/message', { body: data })
    )
  }
}

export const supportService = new SupportService()
