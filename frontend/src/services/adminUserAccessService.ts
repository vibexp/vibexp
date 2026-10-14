import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// How an instance admin helps a user get in (#1436, page #1443): one-time
// setup and reset links, and removing a password. A `link` delivery returns
// the link once; the caller shows it and keeps it nowhere.
export type AdminUserSetupLink = components['schemas']['AdminUserSetupLink']
export type AdminUserSetupLinkRequest =
  components['schemas']['AdminUserSetupLinkRequest']
export type AdminUserResetLink = components['schemas']['AdminUserResetLink']
export type AdminUserResetLinkRequest =
  components['schemas']['AdminUserResetLinkRequest']

class AdminUserAccessService {
  /**
   * A new setup link for an account with no way to sign in yet. Any earlier
   * link stops working.
   */
  async issueSetupLink(
    id: string,
    body: AdminUserSetupLinkRequest
  ): Promise<AdminUserSetupLink> {
    return unwrap(
      generatedClient.POST('/api/v1/admin/users/{id}/setup-link', {
        params: { path: { id } },
        body,
      })
    )
  }

  /**
   * A one-time link to set a new password. With `invalidate_now` the current
   * password is disabled, every session ends and every earlier link dies
   * first.
   */
  async issueResetLink(
    id: string,
    body: AdminUserResetLinkRequest
  ): Promise<AdminUserResetLink> {
    return unwrap(
      generatedClient.POST('/api/v1/admin/users/{id}/reset-link', {
        params: { path: { id } },
        body,
      })
    )
  }

  /**
   * Remove a user's password, disabled or not. 409 `last_sign_in_method`
   * unless `confirmLastMethod` when it is their only way in.
   */
  async removePassword(id: string, confirmLastMethod: boolean): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/admin/users/{id}/password', {
        params: {
          path: { id },
          query: { confirm_last_method: confirmLastMethod },
        },
      })
    )
  }
}

export const adminUserAccessService = new AdminUserAccessService()
