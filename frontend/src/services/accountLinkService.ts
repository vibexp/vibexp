import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// One-time account links (#1432, pages #1439 and #1440). The token travels in
// a POST body only, never in a URL: the page reads it from the link's
// fragment (`features/password/accountLinkToken.ts`).
export type AccountLinkPreview = components['schemas']['AccountLinkPreview']
export type CompleteAccountSetupRequest =
  components['schemas']['CompleteAccountSetupRequest']

class AccountLinkService {
  /**
   * What the link does and whose account it is. 401 for any link that cannot
   * be used; 404 while password sign-in is off.
   */
  async preview(token: string): Promise<AccountLinkPreview> {
    return unwrap(
      generatedClient.POST('/api/v1/auth/password/token/preview', {
        body: { token },
      })
    )
  }

  /**
   * Sets the first password (and the name, when given) and signs the person
   * in. A 503 stored nothing and the link still works; a 500 means the
   * password WAS set but the request did not finish.
   */
  async completeSetup(body: CompleteAccountSetupRequest): Promise<void> {
    await unwrap(generatedClient.POST('/api/v1/auth/password/setup', { body }))
  }
}

export const accountLinkService = new AccountLinkService()
