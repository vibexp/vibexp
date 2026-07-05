import type {
  ActivitiesResponse,
  ActivityFilters,
  ActivityStatsApiResponse,
  ActivityTypesApiResponse,
} from '../activityService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { activityService } from '../activityService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const mockActivity = {
  id: 'activity-1',
  user_id: 'user-1',
  activity_type: 'auth_login',
  entity_type: 'user',
  entity_id: 'user-1',
  session_id: 'session-1',
  description: 'User logged in successfully',
  metadata: { provider: 'google' },
  created_at: '2025-09-27T10:00:00Z',
}

const mockActivitiesResponse: ActivitiesResponse = {
  status: 'success',
  message: 'Activities retrieved successfully',
  data: {
    activities: [mockActivity],
    total_count: 1,
    page: 1,
    per_page: 25,
    total_pages: 1,
  },
}

describe('ActivityService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getActivities', () => {
    it('fetches activities with no filters', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockActivitiesResponse))

      const result = await activityService.getActivities()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/activities',
        { params: { query: {} } }
      )
      expect(result).toEqual(mockActivitiesResponse)
    })

    it('passes filters through as query params', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockActivitiesResponse))
      const filters: ActivityFilters = {
        activity_type: 'auth_login',
        entity_type: 'user',
        search: 'login',
        page: 2,
        limit: 20,
      }

      await activityService.getActivities(filters)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/activities',
        { params: { query: filters } }
      )
    })

    it('throws ApiError with backend detail on RFC 9457 error', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/BAD_REQUEST',
            title: 'Bad Request',
            status: 400,
            detail: 'limit must be an integer between 1 and 100',
            code: 'BAD_REQUEST',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 400, statusText: 'Bad Request' },
        })
      )

      await expect(activityService.getActivities()).rejects.toThrow(
        'limit must be an integer between 1 and 100'
      )
    })
  })

  describe('getActivityStats', () => {
    it('fetches the stats envelope', async () => {
      const statsResponse: ActivityStatsApiResponse = {
        status: 'success',
        message: 'Activity stats retrieved successfully',
        data: {
          total_activities: 100,
          activities_today: 5,
          activities_this_week: 25,
          top_activity_types: [{ activity_type: 'auth_login', count: 30 }],
          top_entity_types: [{ entity_type: 'user', count: 40 }],
          recent_activities: [mockActivity],
          activities_by_date_week: [{ date: '2025-09-27', count: 5 }],
        },
      }
      mockGeneratedClient.GET.mockReturnValue(success(statsResponse))

      const result = await activityService.getActivityStats()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/activities/stats'
      )
      expect(result).toEqual(statsResponse)
    })
  })

  describe('getActivityAndEntityTypes', () => {
    it('fetches the activity/entity types envelope', async () => {
      const typesResponse: ActivityTypesApiResponse = {
        status: 'success',
        message: 'Activity and entity types retrieved successfully',
        data: {
          activity_types: ['auth_login', 'prompt_created'],
          entity_types: ['user', 'prompt'],
        },
      }
      mockGeneratedClient.GET.mockReturnValue(success(typesResponse))

      const result = await activityService.getActivityAndEntityTypes()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/activities/types'
      )
      expect(result).toEqual(typesResponse)
    })

    it('propagates errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: undefined,
          response: {
            ok: false,
            status: 503,
            statusText: 'Service Unavailable',
          },
        })
      )

      await expect(activityService.getActivityAndEntityTypes()).rejects.toThrow(
        'HTTP 503 error'
      )
    })
  })
})
