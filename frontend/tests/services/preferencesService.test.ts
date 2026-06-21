import type {
  PreferencesResponse,
  UpdatePreferencesRequest,
} from '../../src/types/preferences'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  put: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import after mocking
import { preferencesService } from '../../src/services/preferencesService'

describe('PreferencesService', () => {
  const mockPreferences: PreferencesResponse = {
    preferences: {
      email_notification: {
        platform_announcement: true,
        account_security: true,
        new_feature: true,
        marketing_promotional: false,
      },
    },
    updated_at: '2024-01-01T00:00:00Z',
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getPreferences', () => {
    it('should fetch user preferences', async () => {
      mockApiClient.get.mockResolvedValue(mockPreferences)

      const result = await preferencesService.getPreferences()

      expect(mockApiClient.get).toHaveBeenCalledWith('/preferences')
      expect(result).toEqual(mockPreferences)
    })

    it('should handle API errors', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Failed to fetch'))

      await expect(preferencesService.getPreferences()).rejects.toThrow(
        'Failed to fetch'
      )
    })
  })

  describe('updatePreferences', () => {
    it('should update user preferences', async () => {
      const updateRequest: UpdatePreferencesRequest = {
        email_notification: {
          platform_announcement: false,
          account_security: true,
          new_feature: false,
          marketing_promotional: false,
        },
      }

      const updatedPreferences: PreferencesResponse = {
        preferences: {
          email_notification: {
            platform_announcement: false,
            account_security: true,
            new_feature: false,
            marketing_promotional: false,
          },
        },
        updated_at: '2024-01-02T00:00:00Z',
      }

      mockApiClient.put.mockResolvedValue(updatedPreferences)

      const result = await preferencesService.updatePreferences(updateRequest)

      expect(mockApiClient.put).toHaveBeenCalledWith(
        '/preferences',
        updateRequest
      )
      expect(result.preferences.email_notification.platform_announcement).toBe(
        false
      )
    })

    it('should handle partial updates', async () => {
      const updateRequest: UpdatePreferencesRequest = {
        email_notification: {
          platform_announcement: true,
          account_security: true,
          new_feature: true,
          marketing_promotional: true,
        },
      }

      const updatedPreferences: PreferencesResponse = {
        preferences: {
          email_notification: {
            platform_announcement: true,
            account_security: true,
            new_feature: true,
            marketing_promotional: true,
          },
        },
        updated_at: '2024-01-02T00:00:00Z',
      }

      mockApiClient.put.mockResolvedValue(updatedPreferences)

      const result = await preferencesService.updatePreferences(updateRequest)

      expect(mockApiClient.put).toHaveBeenCalledWith(
        '/preferences',
        updateRequest
      )
      expect(result.preferences.email_notification.marketing_promotional).toBe(
        true
      )
    })

    it('should handle API errors', async () => {
      mockApiClient.put.mockRejectedValue(new Error('Failed to update'))

      await expect(
        preferencesService.updatePreferences({
          email_notification: {
            platform_announcement: false,
            account_security: false,
            new_feature: false,
            marketing_promotional: false,
          },
        })
      ).rejects.toThrow('Failed to update')
    })
  })
})
