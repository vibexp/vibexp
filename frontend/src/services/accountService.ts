import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// The signed-in user's own account (#1437, page #1441): name, password and the
// identity providers it can sign in with.
//
// Setting or removing a password and disconnecting an identity end the user's
// OTHER browser sessions and re-issue this one's cookie in the response. A
// request that still carries the old cookie and reaches the server after that
// gets a 401 that signs this browser out, so a caller awaits each of them with
// nothing else in flight and re-reads the account only afterwards.
export type Account = components['schemas']['Account']
export type AccountIdentity = components['schemas']['AccountIdentity']
export type AccountPasswordRequest =
  components['schemas']['AccountPasswordRequest']

const ACCOUNT_PATH = '/api/v1/account'
const PASSWORD_PATH = '/api/v1/account/password'

class AccountService {
  async get(): Promise<Account> {
    return unwrap(generatedClient.GET(ACCOUNT_PATH))
  }

  /** Change the display name; the email cannot be changed. */
  async updateName(name: string): Promise<Account> {
    return unwrap(generatedClient.PATCH(ACCOUNT_PATH, { body: { name } }))
  }

  /**
   * Set a first password or change the existing one (which needs
   * `current_password`). Ends the other browser sessions.
   */
  async setPassword(body: AccountPasswordRequest): Promise<void> {
    await unwrap(generatedClient.PUT(PASSWORD_PATH, { body }))
  }

  /** Remove the password. Ends the other browser sessions. */
  async removePassword(): Promise<void> {
    await unwrap(generatedClient.DELETE(PASSWORD_PATH))
  }

  /** Disconnect one identity provider. Ends the other browser sessions. */
  async disconnectIdentity(id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/account/identities/{id}', {
        params: { path: { id } },
      })
    )
  }
}

export const accountService = new AccountService()
