import { act, renderHook, waitFor } from '@testing-library/react'

// Mock notificationService
const mockNotificationService = {
  getUnreadCount: jest.fn(),
}

jest.mock('../../services/notificationService', () => ({
  notificationService: mockNotificationService,
}))

import { useUnreadCount } from '../useUnreadCount'

describe('useUnreadCount', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    // Reset visibility state
    Object.defineProperty(document, 'visibilityState', {
      value: 'visible',
      configurable: true,
    })
  })

  it('fetches unread count on mount', async () => {
    mockNotificationService.getUnreadCount.mockResolvedValue({
      unread_count: 5,
    })

    const { result } = renderHook(() => useUnreadCount())

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(5)
    })

    expect(mockNotificationService.getUnreadCount).toHaveBeenCalled()
  })

  it('starts with zero unread count before fetch completes', () => {
    mockNotificationService.getUnreadCount.mockReturnValue(
      new Promise(() => {})
    )

    const { result } = renderHook(() => useUnreadCount())

    expect(result.current.unreadCount).toBe(0)
  })

  it('logs a warning on error without changing UI state', async () => {
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {})
    mockNotificationService.getUnreadCount.mockRejectedValue(
      new Error('Network error')
    )

    const { result } = renderHook(() => useUnreadCount())

    await waitFor(() => {
      expect(mockNotificationService.getUnreadCount).toHaveBeenCalled()
    })

    expect(result.current.unreadCount).toBe(0)
    expect(warnSpy).toHaveBeenCalledWith(
      '[useUnreadCount] fetch failed',
      expect.any(Error)
    )
    warnSpy.mockRestore()
  })

  it('incrementUnread increases count by 1', async () => {
    mockNotificationService.getUnreadCount.mockResolvedValue({
      unread_count: 3,
    })

    const { result } = renderHook(() => useUnreadCount())

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(3)
    })

    act(() => {
      result.current.incrementUnread()
    })

    expect(result.current.unreadCount).toBe(4)
  })

  it('resetUnread sets count to zero', async () => {
    mockNotificationService.getUnreadCount.mockResolvedValue({
      unread_count: 7,
    })

    const { result } = renderHook(() => useUnreadCount())

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(7)
    })

    act(() => {
      result.current.resetUnread()
    })

    expect(result.current.unreadCount).toBe(0)
  })

  it('refresh triggers a new fetch', async () => {
    mockNotificationService.getUnreadCount
      .mockResolvedValueOnce({ unread_count: 2 })
      .mockResolvedValueOnce({ unread_count: 8 })

    const { result } = renderHook(() => useUnreadCount())

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(2)
    })

    act(() => {
      result.current.refresh()
    })

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(8)
    })

    expect(mockNotificationService.getUnreadCount).toHaveBeenCalledTimes(2)
  })

  it('cleans up on unmount', () => {
    mockNotificationService.getUnreadCount.mockResolvedValue({
      unread_count: 0,
    })

    const { unmount } = renderHook(() => useUnreadCount())

    // Should not throw
    expect(() => {
      unmount()
    }).not.toThrow()
  })

  it('guards against concurrent fetches — only one network call fires at a time', async () => {
    let resolveFirst!: (v: { unread_count: number }) => void

    // First call hangs until we manually resolve it
    mockNotificationService.getUnreadCount.mockReturnValueOnce(
      new Promise<{ unread_count: number }>(resolve => {
        resolveFirst = resolve
      })
    )
    mockNotificationService.getUnreadCount.mockResolvedValue({
      unread_count: 10,
    })

    const { result } = renderHook(() => useUnreadCount())

    // Trigger a second fetch via refresh while the first is still in flight
    act(() => {
      result.current.refresh()
    })

    // At this point only 1 call should have been made (second was guarded)
    expect(mockNotificationService.getUnreadCount).toHaveBeenCalledTimes(1)

    // Resolve first call
    act(() => {
      resolveFirst({ unread_count: 5 })
    })

    await waitFor(() => {
      expect(result.current.unreadCount).toBe(5)
    })

    // Still only 1 call total - the second was blocked by in-flight guard
    expect(mockNotificationService.getUnreadCount).toHaveBeenCalledTimes(1)
  })

  it('does not update state when tab is hidden', async () => {
    // Mark tab as hidden before fetch resolves
    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      configurable: true,
    })

    let resolveCall!: (v: { unread_count: number }) => void
    mockNotificationService.getUnreadCount.mockReturnValueOnce(
      new Promise<{ unread_count: number }>(resolve => {
        resolveCall = resolve
      })
    )

    const { result } = renderHook(() => useUnreadCount())

    act(() => {
      resolveCall({ unread_count: 42 })
    })

    // State should not be updated because tab is hidden
    await waitFor(() => {
      expect(mockNotificationService.getUnreadCount).toHaveBeenCalled()
    })

    expect(result.current.unreadCount).toBe(0)
  })
})
