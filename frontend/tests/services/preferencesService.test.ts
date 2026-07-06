import type {
  PreferencesResponse,
  UpdatePreferencesRequest,
} from '../../src/services/preferencesService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  PUT: jest.fn(),
}

jest.mock('../../src/lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../src/lib/apiClientGenerated')
  >('../../src/lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

// Import after mocking
import { preferencesService } from '../../src/services/preferencesService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('PreferencesService', () => {
  const mockPreferences: PreferencesResponse = {
    preferences: {
      email_notification: {
        platform_announcement: true,
        account_security: true,
        new_feature: true,
        marketing_promotional: false,
      },
      notifications: {
        channels: { in_app: true, email: true, web_push: false },
        types: {},
      },
    },
    updated_at: '2024-01-01T00:00:00Z',
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getPreferences', () => {
    it('should fetch user preferences', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockPreferences))

      const result = await preferencesService.getPreferences()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/preferences'
      )
      expect(result).toEqual(mockPreferences)
    })

    it('should handle API errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
            title: 'Internal Server Error',
            status: 500,
            detail: 'Failed to fetch',
            code: 'INTERNAL_ERROR',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 500, statusText: 'Server Error' },
        })
      )

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
          notifications: {
            channels: { in_app: true, email: true, web_push: false },
            types: {},
          },
        },
        updated_at: '2024-01-02T00:00:00Z',
      }

      mockGeneratedClient.PUT.mockReturnValue(success(updatedPreferences))

      const result = await preferencesService.updatePreferences(updateRequest)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/preferences',
        {
          body: updateRequest,
        }
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
          notifications: {
            channels: { in_app: true, email: true, web_push: false },
            types: {},
          },
        },
        updated_at: '2024-01-02T00:00:00Z',
      }

      mockGeneratedClient.PUT.mockReturnValue(success(updatedPreferences))

      const result = await preferencesService.updatePreferences(updateRequest)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/preferences',
        {
          body: updateRequest,
        }
      )
      expect(result.preferences.email_notification.marketing_promotional).toBe(
        true
      )
    })

    it('should handle API errors', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
            title: 'Internal Server Error',
            status: 500,
            detail: 'Failed to update',
            code: 'INTERNAL_ERROR',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 500, statusText: 'Server Error' },
        })
      )

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
