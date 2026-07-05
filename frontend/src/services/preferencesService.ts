import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the user-preferences domain — the OpenAPI spec is
// the single source of truth; do not hand-write request/response shapes here.
export type EmailNotificationPreferences =
  components['schemas']['EmailNotificationPreferences']
export type NotificationTypePreference =
  components['schemas']['NotificationTypePreference']
export type NotificationChannelPreferences =
  components['schemas']['NotificationChannelPreferences']
export type NotificationPreferences =
  components['schemas']['NotificationPreferences']
export type Preferences = components['schemas']['Preferences']
export type PreferencesResponse = components['schemas']['PreferencesResponse']
export type UpdatePreferencesRequest =
  components['schemas']['UpdatePreferencesRequest']

class PreferencesService {
  async getPreferences(): Promise<PreferencesResponse> {
    return unwrap(generatedClient.GET('/api/v1/preferences'))
  }

  async updatePreferences(
    request: UpdatePreferencesRequest
  ): Promise<PreferencesResponse> {
    return unwrap(generatedClient.PUT('/api/v1/preferences', { body: request }))
  }
}

export const preferencesService = new PreferencesService()
