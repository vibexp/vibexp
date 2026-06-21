import { apiClient } from '../lib/apiClient'
import type {
  PreferencesResponse,
  UpdatePreferencesRequest,
} from '../types/preferences'

class PreferencesService {
  async getPreferences(): Promise<PreferencesResponse> {
    return apiClient.get<PreferencesResponse>('/preferences')
  }

  async updatePreferences(
    request: UpdatePreferencesRequest
  ): Promise<PreferencesResponse> {
    return apiClient.put<PreferencesResponse>('/preferences', request)
  }
}

export const preferencesService = new PreferencesService()
