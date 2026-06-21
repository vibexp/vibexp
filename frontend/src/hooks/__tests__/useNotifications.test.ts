import { act, renderHook, waitFor } from '@testing-library/react'

import type {
  Notification,
  NotificationListResponse,
} from '@/services/notificationService'

// Mock notificationService
const mockNotificationService = {
  listNotifications: jest.fn(),
  getUnreadCount: jest.fn(),
  markAsRead: jest.fn(),
  markAllAsRead: jest.fn(),
}

jest.mock('../../services/notificationService', () => ({
  notificationService: mockNotificationService,
}))

// Mock toast
const mockToastError = jest.fn()
jest.mock('../../lib/toast', () => ({
  toast: {
    error: (...args: unknown[]) => mockToastError(...args),
  },
}))

import { useNotifications } from '../useNotifications'

const makeNotification = (
  id: string,
  read = false,
  overrides: Partial<Notification> = {}
): Notification => ({
  id,
  type: 'feed.item.created',
  category: 'low',
  title: `Notification ${id}`,
  body: 'Some body text',
  action_url: '/feeds/1',
  created_at: '2024-01-01T10:00:00Z',
  ...(read ? { read_at: '2024-01-01T11:00:00Z' } : {}),
  ...overrides,
})

const makeResponse = (
  notifications: Notification[],
  { limit = 20, offset = 0 }: { limit?: number; offset?: number } = {}
): NotificationListResponse => ({
  notifications,
  count: notifications.length,
  limit,
  offset,
})

describe('useNotifications', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches notifications on mount', async () => {
    const notifications = [makeNotification('1'), makeNotification('2')]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )

    const { result } = renderHook(() => useNotifications())

    expect(result.current.loading).toBe(true)

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.notifications).toHaveLength(2)
    expect(result.current.error).toBeNull()
  })

  it('sets error on fetch failure', async () => {
    mockNotificationService.listNotifications.mockRejectedValue(
      new Error('Network error')
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.error).toBe('Network error')
    expect(result.current.notifications).toHaveLength(0)
  })

  it('marks a notification as read optimistically', async () => {
    const notifications = [
      makeNotification('1', false),
      makeNotification('2', true),
    ]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )
    mockNotificationService.markAsRead.mockResolvedValue(undefined)

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.markAsRead('1')
    })

    expect(result.current.notifications[0].read_at).toBeTruthy()
    expect(mockNotificationService.markAsRead).toHaveBeenCalledWith('1')
  })

  it('rolls back markAsRead on API failure and shows toast', async () => {
    const notifications = [makeNotification('1', false)]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )
    mockNotificationService.markAsRead.mockRejectedValue(
      new Error('Server error')
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.markAsRead('1')
    })

    // After optimistic update notification should be read
    expect(result.current.notifications[0].read_at).toBeTruthy()

    // After rollback, notification should be unread again
    await waitFor(() => {
      expect(result.current.notifications[0].read_at).toBeUndefined()
    })

    expect(mockToastError).toHaveBeenCalled()
  })

  it('marks all notifications as read optimistically', async () => {
    const notifications = [
      makeNotification('1', false),
      makeNotification('2', false),
    ]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )
    mockNotificationService.markAllAsRead.mockResolvedValue(undefined)

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.markAllAsRead()
    })

    expect(result.current.notifications.every(n => n.read_at)).toBe(true)
    expect(mockNotificationService.markAllAsRead).toHaveBeenCalled()
  })

  it('rolls back markAllAsRead on API failure and shows toast', async () => {
    const notifications = [
      makeNotification('1', false),
      makeNotification('2', false),
    ]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )
    mockNotificationService.markAllAsRead.mockRejectedValue(
      new Error('Server error')
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.markAllAsRead()
    })

    // After optimistic update all should be read
    expect(result.current.notifications.every(n => n.read_at)).toBe(true)

    // After rollback, notifications should be unread again
    await waitFor(() => {
      expect(result.current.notifications.every(n => !n.read_at)).toBe(true)
    })

    expect(mockToastError).toHaveBeenCalled()
  })

  it('reports hasMore when the page is full', async () => {
    const fullPage = Array.from({ length: 20 }, (_, i) =>
      makeNotification(String(i + 1))
    )
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(fullPage)
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.hasMore).toBe(true)
  })

  it('fetches the next offset page on fetchMore', async () => {
    const firstPage = [makeNotification('1'), makeNotification('2')]
    const secondPage = [makeNotification('3')]

    mockNotificationService.listNotifications
      .mockResolvedValueOnce(makeResponse(firstPage, { limit: 2 }))
      .mockResolvedValueOnce(makeResponse(secondPage, { limit: 2, offset: 2 }))

    const { result } = renderHook(() => useNotifications({ limit: 2 }))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.notifications).toHaveLength(2)
    expect(result.current.hasMore).toBe(true)

    act(() => {
      result.current.fetchMore()
    })

    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(3)
    })

    expect(mockNotificationService.listNotifications).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 2 })
    )
    expect(result.current.hasMore).toBe(false)
  })

  it('deduplicates items returned twice across offset pages', async () => {
    const firstPage = [makeNotification('1'), makeNotification('2')]
    // A new notification shifted offsets, so '2' comes back on page 2 too
    const secondPage = [makeNotification('2'), makeNotification('3')]

    mockNotificationService.listNotifications
      .mockResolvedValueOnce(makeResponse(firstPage, { limit: 2 }))
      .mockResolvedValueOnce(makeResponse(secondPage, { limit: 2, offset: 2 }))

    const { result } = renderHook(() => useNotifications({ limit: 2 }))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.fetchMore()
    })

    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(3)
    })

    expect(result.current.notifications.map(n => n.id)).toEqual(['1', '2', '3'])
  })

  it('guards against concurrent fetchMore calls', async () => {
    let resolveFirst!: (v: NotificationListResponse) => void
    const firstPage = Array.from({ length: 20 }, (_, i) =>
      makeNotification(String(i + 1))
    )

    mockNotificationService.listNotifications.mockResolvedValueOnce(
      makeResponse(firstPage)
    )
    mockNotificationService.listNotifications.mockReturnValueOnce(
      new Promise<NotificationListResponse>(resolve => {
        resolveFirst = resolve
      })
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    // Trigger two fetchMore calls in rapid succession
    act(() => {
      result.current.fetchMore()
      result.current.fetchMore()
    })

    // Only one additional network call should be made
    expect(mockNotificationService.listNotifications).toHaveBeenCalledTimes(2)

    act(() => {
      resolveFirst(makeResponse([makeNotification('21')], { offset: 20 }))
    })

    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(21)
    })
  })

  it('refresh resets the offset and refetches', async () => {
    const notifications = [makeNotification('1')]
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse(notifications)
    )

    const { result } = renderHook(() => useNotifications())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.refresh()
    })

    await waitFor(() => {
      expect(mockNotificationService.listNotifications).toHaveBeenCalledTimes(2)
    })

    expect(mockNotificationService.listNotifications).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 0 })
    )
  })

  it('passes unread filter param to service', async () => {
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse([])
    )

    renderHook(() => useNotifications({ unread: true, limit: 10 }))

    await waitFor(() => {
      expect(mockNotificationService.listNotifications).toHaveBeenCalledWith(
        expect.objectContaining({ unread: true, limit: 10 })
      )
    })
  })

  it('re-fetches with unread=true when filter changes to unread', async () => {
    mockNotificationService.listNotifications.mockResolvedValue(
      makeResponse([])
    )

    const { rerender } = renderHook(
      ({ unread }: { unread: boolean | undefined }) =>
        useNotifications({ limit: 20, unread }),
      { initialProps: { unread: undefined as boolean | undefined } }
    )

    await waitFor(() => {
      expect(mockNotificationService.listNotifications).toHaveBeenCalledTimes(1)
    })

    rerender({ unread: true })

    await waitFor(() => {
      expect(mockNotificationService.listNotifications).toHaveBeenCalledTimes(2)
      expect(
        mockNotificationService.listNotifications
      ).toHaveBeenLastCalledWith(
        expect.objectContaining({ unread: true, limit: 20 })
      )
    })
  })
})
