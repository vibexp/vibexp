import { apiClient } from '../lib/apiClient'
import type { SupportRequestData, SupportResponse } from '../types'

class SupportService {
  async submitSupportRequest(
    data: SupportRequestData
  ): Promise<SupportResponse> {
    return apiClient.post<SupportResponse>('/support/message', data)
  }
}

export const supportService = new SupportService()
