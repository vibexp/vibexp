import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { toast } from '@/lib/toast'
import {
  type Notification,
  notificationService,
} from '@/services/notificationService'

export interface UseNotificationsParams {
  unread?: boolean
  limit?: number
}

export interface UseNotificationsResult {
  notifications: Notification[]
  loading: boolean
  error: string | null
  hasMore: boolean
  fetchMore: () => void
  markAsRead: (id: string) => void
  markAllAsRead: () => void
  refresh: () => void
}

export function useNotifications(
  params: UseNotificationsParams = {}
): UseNotificationsResult {
  const { unread, limit = 20 } = params

  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const offsetRef = useRef(0)
  const fetchingRef = useRef(false)

  // Keep a stable ref to the latest params so fetch callbacks can always read
  // the current values without being listed as deps on every downstream hook.
  const paramsRef = useRef({ unread, limit })
  paramsRef.current = { unread, limit }

  const fetchNotifications = useCallback(async (reset: boolean) => {
    if (fetchingRef.current) return
    fetchingRef.current = true
    setLoading(true)
    setError(null)
    try {
      const response = await notificationService.listNotifications({
        unread: paramsRef.current.unread,
        limit: paramsRef.current.limit,
        offset: reset ? 0 : offsetRef.current,
      })
      // Dedupe on append: items arriving between pages shift offsets, so a
      // boundary notification can be returned twice (newest-first list).
      setNotifications(prev =>
        reset
          ? response.notifications
          : [
              ...prev,
              ...response.notifications.filter(
                n => !prev.some(p => p.id === n.id)
              ),
            ]
      )
      offsetRef.current = response.offset + response.count
      // The API reports no global total; a full page means more may exist.
      setHasMore(response.count === response.limit)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load notifications'
      )
    } finally {
      setLoading(false)
      fetchingRef.current = false
    }
  }, [])

  // Re-fetch from page 1 whenever filter params change
  useEffect(() => {
    offsetRef.current = 0
    void fetchNotifications(true)
  }, [unread, limit, fetchNotifications])

  const fetchMore = useCallback(() => {
    if (hasMore && !loading) {
      void fetchNotifications(false)
    }
  }, [hasMore, loading, fetchNotifications])

  const refresh = useCallback(() => {
    offsetRef.current = 0
    void fetchNotifications(true)
  }, [fetchNotifications])

  const markAsRead = useCallback(
    (id: string) => {
      const snapshot = notifications.slice()
      const readAt = new Date().toISOString()
      setNotifications(prev =>
        prev.map(n => (n.id === id ? { ...n, read_at: readAt } : n))
      )
      void notificationService.markAsRead(id).catch((err: unknown) => {
        setNotifications(snapshot)
        toast.error('Failed to mark notification as read', {
          description: err instanceof Error ? err.message : undefined,
        })
      })
    },
    [notifications]
  )

  const markAllAsRead = useCallback(() => {
    const snapshot = notifications.slice()
    const readAt = new Date().toISOString()
    setNotifications(prev =>
      prev.map(n => (n.read_at ? n : { ...n, read_at: readAt }))
    )
    void notificationService.markAllAsRead().catch((err: unknown) => {
      setNotifications(snapshot)
      toast.error('Failed to mark all notifications as read', {
        description: err instanceof Error ? err.message : undefined,
      })
    })
  }, [notifications])

  return useMemo(
    () => ({
      notifications,
      loading,
      error,
      hasMore,
      fetchMore,
      markAsRead,
      markAllAsRead,
      refresh,
    }),
    [
      notifications,
      loading,
      error,
      hasMore,
      fetchMore,
      markAsRead,
      markAllAsRead,
      refresh,
    ]
  )
}
