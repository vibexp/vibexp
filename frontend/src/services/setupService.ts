import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// First-run authentication setup (#1236, page #1239). Both calls are public:
// the status is what the sign-in page reads, and the session exchange is what
// turns the one-time setup token into the `vibexp_setup` cookie.
export type SetupStatus = components['schemas']['SetupStatusResponse']
export type SetupSession = components['schemas']['SetupSessionResponse']

class SetupService {
  /** Whether the instance still has to be configured through `/setup`. */
  async getStatus(): Promise<SetupStatus> {
    return unwrap(generatedClient.GET('/api/v1/setup/status'))
  }

  /**
   * Exchange the setup token for a setup session cookie. 401 for an invalid
   * or expired token, 404 when the instance is not in setup mode.
   */
  async createSession(token: string): Promise<SetupSession> {
    return unwrap(
      generatedClient.POST('/api/v1/setup/session', { body: { token } })
    )
  }
}

export const setupService = new SetupService()
