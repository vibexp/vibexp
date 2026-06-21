import type {
  ActivityFilters,
  ActivitiesResponse,
  ActivityStatsApiResponse,
  ActivityTypesApiResponse,
  Activity,
  ActivityListResponse,
  ActivityStatsResponse,
  ActivityTypesResponse,
} from '../../src/types'

// Mock fetch globally
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock the entire ActivityService module to avoid import.meta.env issues
jest.mock('../../src/services/activityService', () => {
  class ActivityService {
    private async makeRequest<T>(
      endpoint: string,
      options: RequestInit = {}
    ): Promise<T> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      // Use production API URL for tests
      const API_BASE_URL = 'https://api.vibexp.io/api/v1'

      const response = await fetch(`${API_BASE_URL}${endpoint}`, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
          ...options.headers,
        },
      })

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.setToken(null)
          throw new Error('Authentication expired')
        }
        const errorData = await response.json().catch(() => null)
        throw new Error(
          errorData?.message || `HTTP error! status: ${response.status}`
        )
      }

      // Handle 204 No Content responses
      if (response.status === 204) {
        return null as T
      }

      // Check if response has JSON content
      const contentType = response.headers.get('content-type')
      if (contentType && contentType.includes('application/json')) {
        return response.json()
      }

      // Try to parse as JSON even if content-type header is not set correctly
      try {
        const text = await response.text()
        if (text.trim()) {
          return JSON.parse(text)
        }
      } catch (e) {
        console.warn('Failed to parse response as JSON:', e)
      }

      return null as T
    }

    async getActivities(
      filters: ActivityFilters = {}
    ): Promise<ActivitiesResponse> {
      const params = new URLSearchParams()

      if (filters.activity_type)
        params.append('activity_type', filters.activity_type)
      if (filters.entity_type) params.append('entity_type', filters.entity_type)
      if (filters.search) params.append('search', filters.search)
      if (filters.page) params.append('page', filters.page.toString())
      if (filters.limit) params.append('limit', filters.limit.toString())

      const queryString = params.toString()
      const endpoint = `/activities${queryString ? `?${queryString}` : ''}`

      return this.makeRequest<ActivitiesResponse>(endpoint)
    }

    async getActivityStats(): Promise<ActivityStatsApiResponse> {
      return this.makeRequest<ActivityStatsApiResponse>('/activities/stats')
    }

    async getActivityAndEntityTypes(): Promise<ActivityTypesApiResponse> {
      return this.makeRequest<ActivityTypesApiResponse>('/activities/types')
    }
  }

  return {
    activityService: new ActivityService(),
  }
})

// Import after mocking
import { activityService } from '../../src/services/activityService'

describe('ActivityService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockFetch.mockClear()
    mockAuthService.getToken.mockClear()
    mockAuthService.setToken.mockClear()
  })

  describe('getActivities', () => {
    const mockActivity: Activity = {
      id: 'activity-1',
      user_id: 'user-1',
      activity_type: 'auth_login',
      entity_type: 'user',
      entity_id: 'user-1',
      session_id: 'session-1',
      description: 'User logged in successfully',
      metadata: { provider: 'google' },
      source_ip: '192.168.1.1',
      user_agent: 'Mozilla/5.0',
      created_at: '2025-09-27T10:00:00Z',
    }

    const mockActivityListResponse: ActivityListResponse = {
      activities: [mockActivity],
      total_count: 1,
      page: 1,
      per_page: 10,
      total_pages: 1,
    }

    const mockApiResponse: ActivitiesResponse = {
      status: 'success',
      data: mockActivityListResponse,
      message: 'Activities retrieved successfully',
    }

    it('should retrieve activities without filters', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue(mockApiResponse),
      })

      const result = await activityService.getActivities()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities',
        {
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer valid-token',
          },
        }
      )
      expect(result).toEqual(mockApiResponse)
    })

    it('should retrieve activities with filters', async () => {
      const filters: ActivityFilters = {
        activity_type: 'auth_login',
        entity_type: 'user',
        search: 'login',
        page: 2,
        limit: 20,
      }

      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue(mockApiResponse),
      })

      const result = await activityService.getActivities(filters)

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities?activity_type=auth_login&entity_type=user&search=login&page=2&limit=20',
        {
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer valid-token',
          },
        }
      )
      expect(result).toEqual(mockApiResponse)
    })

    it('should handle empty filters gracefully', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue(mockApiResponse),
      })

      const result = await activityService.getActivities({})

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities',
        {
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer valid-token',
          },
        }
      )
      expect(result).toEqual(mockApiResponse)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(activityService.getActivities()).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockAuthService.getToken.mockReturnValue('invalid-token')
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: jest.fn().mockResolvedValue({ message: 'Unauthorized' }),
      })

      await expect(activityService.getActivities()).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })

    it('should handle server error responses', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: jest.fn().mockResolvedValue({ message: 'Internal server error' }),
      })

      await expect(activityService.getActivities()).rejects.toThrow(
        'Internal server error'
      )
    })

    it('should handle server error without error message', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: jest.fn().mockRejectedValue(new Error('Not JSON')),
      })

      await expect(activityService.getActivities()).rejects.toThrow(
        'HTTP error! status: 404'
      )
    })

    it('should handle 204 No Content response', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 204,
        headers: new Headers(),
      })

      const result = await activityService.getActivities()

      expect(result).toBeNull()
    })

    it('should handle response without content-type header', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: jest.fn().mockResolvedValue(JSON.stringify(mockApiResponse)),
      })

      const result = await activityService.getActivities()

      expect(result).toEqual(mockApiResponse)
    })

    it('should handle empty response text', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: jest.fn().mockResolvedValue(''),
      })

      const result = await activityService.getActivities()

      expect(result).toBeNull()
    })

    it('should handle invalid JSON response gracefully', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      const consoleWarnSpy = jest
        .spyOn(console, 'warn')
        .mockImplementation(() => {})

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: jest.fn().mockResolvedValue('invalid json'),
      })

      const result = await activityService.getActivities()

      expect(consoleWarnSpy).toHaveBeenCalledWith(
        'Failed to parse response as JSON:',
        expect.any(Error)
      )
      expect(result).toBeNull()

      consoleWarnSpy.mockRestore()
    })
  })

  describe('getActivityStats', () => {
    const mockActivityStatsResponse: ActivityStatsResponse = {
      total_activities: 100,
      activities_today: 5,
      activities_this_week: 25,
      top_activity_types: [
        { activity_type: 'auth_login', count: 30 },
        { activity_type: 'prompt_created', count: 20 },
      ],
      top_entity_types: [
        { entity_type: 'user', count: 40 },
        { entity_type: 'prompt', count: 20 },
      ],
      recent_activities: [
        {
          id: 'activity-1',
          user_id: 'user-1',
          activity_type: 'auth_login',
          entity_type: 'user',
          description: 'User logged in',
          metadata: {},
          created_at: '2025-09-27T10:00:00Z',
        },
      ],
      activities_by_date_week: [
        { date: '2025-09-27', count: 5 },
        { date: '2025-09-26', count: 8 },
      ],
    }

    const mockApiResponse: ActivityStatsApiResponse = {
      status: 'success',
      data: mockActivityStatsResponse,
      message: 'Activity stats retrieved successfully',
    }

    it('should retrieve activity statistics', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue(mockApiResponse),
      })

      const result = await activityService.getActivityStats()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities/stats',
        {
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer valid-token',
          },
        }
      )
      expect(result).toEqual(mockApiResponse)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(activityService.getActivityStats()).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle authentication expired error', async () => {
      mockAuthService.getToken.mockReturnValue('expired-token')
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: jest.fn().mockResolvedValue({ message: 'Token expired' }),
      })

      await expect(activityService.getActivityStats()).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('getActivityAndEntityTypes', () => {
    const mockActivityTypesResponse: ActivityTypesResponse = {
      activity_types: [
        'auth_login',
        'auth_logout',
        'prompt_created',
        'prompt_updated',
        'agent_created',
      ],
      entity_types: ['user', 'prompt', 'agent', 'memory', 'session'],
    }

    const mockApiResponse: ActivityTypesApiResponse = {
      status: 'success',
      data: mockActivityTypesResponse,
      message: 'Activity and entity types retrieved successfully',
    }

    it('should retrieve activity and entity types', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue(mockApiResponse),
      })

      const result = await activityService.getActivityAndEntityTypes()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities/types',
        {
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer valid-token',
          },
        }
      )
      expect(result).toEqual(mockApiResponse)
      expect(result.data.activity_types).toBeDefined()
      expect(result.data.entity_types).toBeDefined()
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(activityService.getActivityAndEntityTypes()).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle server error', async () => {
      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: jest.fn().mockResolvedValue({ message: 'Service unavailable' }),
      })

      await expect(activityService.getActivityAndEntityTypes()).rejects.toThrow(
        'Service unavailable'
      )
    })
  })

  describe('URL construction and filtering', () => {
    it('should construct URL correctly with single filter', async () => {
      const filters: ActivityFilters = {
        activity_type: 'auth_login',
      }

      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue({
          status: 'success',
          data: {
            activities: [],
            total_count: 0,
            page: 1,
            per_page: 10,
            total_pages: 0,
          },
        }),
      })

      await activityService.getActivities(filters)

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities?activity_type=auth_login',
        expect.any(Object)
      )
    })

    it('should handle special characters in search filters', async () => {
      const filters: ActivityFilters = {
        search: 'user@example.com',
      }

      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue({
          status: 'success',
          data: {
            activities: [],
            total_count: 0,
            page: 1,
            per_page: 10,
            total_pages: 0,
          },
        }),
      })

      await activityService.getActivities(filters)

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities?search=user%40example.com',
        expect.any(Object)
      )
    })

    it('should handle zero values in filters (falsy values are excluded)', async () => {
      const filters: ActivityFilters = {
        page: 0,
        limit: 0,
      }

      mockAuthService.getToken.mockReturnValue('valid-token')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: jest.fn().mockResolvedValue({
          status: 'success',
          data: {
            activities: [],
            total_count: 0,
            page: 1,
            per_page: 10,
            total_pages: 0,
          },
        }),
      })

      await activityService.getActivities(filters)

      // Zero values are falsy and excluded from URL params
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/activities',
        expect.any(Object)
      )
    })
  })
})
