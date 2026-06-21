import type {
  NotificationListResponse,
  UnreadCountResponse,
} from '../notificationService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  PATCH: jest.fn(),
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

import { notificationService } from '../notificationService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContentResponse = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const mockNotification = {
  id: 'notif-1',
  type: 'feed.item.created',
  category: 'low' as const,
  title: 'New feed item',
  body: 'Someone posted to your feed',
  action_url: '/feeds/1',
  created_at: '2024-01-01T10:00:00Z',
}

describe('NotificationService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('listNotifications', () => {
    it('should fetch notifications with no params', async () => {
      const mockResponse: NotificationListResponse = {
        notifications: [mockNotification],
        count: 1,
        limit: 20,
        offset: 0,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await notificationService.listNotifications()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/notifications',
        { params: { query: {} } }
      )
      expect(result).toEqual(mockResponse)
    })

    it('should pass unread, limit, and offset query params', async () => {
      const mockResponse: NotificationListResponse = {
        notifications: [],
        count: 0,
        limit: 10,
        offset: 20,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      await notificationService.listNotifications({
        unread: true,
        limit: 10,
        offset: 20,
      })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/notifications',
        { params: { query: { unread: true, limit: 10, offset: 20 } } }
      )
    })

    it('should throw ApiError with backend detail on RFC 9457 error', async () => {
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

      await expect(notificationService.listNotifications()).rejects.toThrow(
        'limit must be an integer between 1 and 100'
      )
    })
  })

  describe('getUnreadCount', () => {
    it('should fetch unread count', async () => {
      const mockResponse: UnreadCountResponse = { unread_count: 5 }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await notificationService.getUnreadCount()

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/notifications/unread-count'
      )
      expect(result).toEqual({ unread_count: 5 })
    })

    it('should return zero when no unread notifications', async () => {
      mockGeneratedClient.GET.mockReturnValue(success({ unread_count: 0 }))

      const result = await notificationService.getUnreadCount()

      expect(result.unread_count).toBe(0)
    })

    it('should propagate errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: 'oops',
          response: {
            ok: false,
            status: 500,
            statusText: 'Internal Server Error',
          },
        })
      )

      await expect(notificationService.getUnreadCount()).rejects.toThrow('oops')
    })
  })

  describe('markAsRead', () => {
    it('should call patch endpoint for notification id', async () => {
      mockGeneratedClient.PATCH.mockReturnValue(
        Promise.resolve({ data: undefined, response: noContentResponse })
      )

      await notificationService.markAsRead('notif-1')

      expect(mockGeneratedClient.PATCH).toHaveBeenCalledWith(
        '/api/v1/notifications/{id}/read',
        { params: { path: { id: 'notif-1' } } }
      )
    })

    it('should propagate errors', async () => {
      mockGeneratedClient.PATCH.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/BAD_REQUEST',
            title: 'Bad Request',
            status: 400,
            detail: 'notification id must be a valid UUID',
            code: 'BAD_REQUEST',
            request_id: 'req-2',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 400, statusText: 'Bad Request' },
        })
      )

      await expect(notificationService.markAsRead('bad-id')).rejects.toThrow(
        'notification id must be a valid UUID'
      )
    })
  })

  describe('markAllAsRead', () => {
    it('should call patch read-all endpoint', async () => {
      mockGeneratedClient.PATCH.mockReturnValue(
        Promise.resolve({ data: undefined, response: noContentResponse })
      )

      await notificationService.markAllAsRead()

      expect(mockGeneratedClient.PATCH).toHaveBeenCalledWith(
        '/api/v1/notifications/read-all'
      )
    })

    it('should propagate errors', async () => {
      mockGeneratedClient.PATCH.mockReturnValue(
        Promise.resolve({
          error: undefined,
          response: { ok: false, status: 401, statusText: 'Unauthorized' },
        })
      )

      await expect(notificationService.markAllAsRead()).rejects.toThrow(
        'HTTP 401 error'
      )
    })
  })
})
