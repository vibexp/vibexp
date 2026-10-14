import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// What the holder of an invitation link can do with no account and no session
// (#1435, page #1440): see the invitation, and create an account with a
// password through it. The token is the credential; both calls are public.
export type InvitationPreview = components['schemas']['InvitationPreview']
export type InvitationRegistration =
  components['schemas']['InvitationRegistration']
export type InvitationRegisterRequest =
  components['schemas']['InvitationRegisterRequest']

class PublicInvitationService {
  /** 404 `invitation_invalid` for any link that cannot be used. */
  async preview(token: string): Promise<InvitationPreview> {
    return unwrap(
      generatedClient.GET('/api/v1/invitations/{token}/preview', {
        params: { path: { token } },
      })
    )
  }

  /**
   * Creates the account, accepts the invitation and signs the person in (the
   * response sets the session cookie). `invitation_accepted: false` means the
   * account exists and is signed in but the invitation is still pending.
   */
  async register(
    token: string,
    body: InvitationRegisterRequest
  ): Promise<InvitationRegistration> {
    return unwrap(
      generatedClient.POST('/api/v1/invitations/{token}/register', {
        params: { path: { token } },
        body,
      })
    )
  }
}

export const publicInvitationService = new PublicInvitationService()
