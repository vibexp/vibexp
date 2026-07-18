import { useCallback, useEffect, useRef, useState } from 'react'

import { notificationService } from '@/services/notificationService'

const POLL_INTERVAL_MS = 60_000 // 1 minute on focus

export interface UseUnreadCountResult {
  unreadCount: number
  incrementUnread: () => void
  resetUnread: () => void
  refresh: () => void
}

export function useUnreadCount(): UseUnreadCountResult {
  const [unreadCount, setUnreadCount] = useState(0)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)
  const inFlightRef = useRef(false)

  const fetchCount = useCallback(async () => {
    if (inFlightRef.current) return
    inFlightRef.current = true
    try {
      const response = await notificationService.getUnreadCount()
      if (mountedRef.current && document.visibilityState === 'visible') {
        setUnreadCount(response.unread_count)
      }
    } catch (err) {
      console.warn('[useUnreadCount] fetch failed', err)
    } finally {
      inFlightRef.current = false
    }
  }, [])

  const scheduleNextPoll = useCallback(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
    }
    timeoutRef.current = setTimeout(() => {
      if (document.visibilityState === 'visible' && mountedRef.current) {
        void fetchCount()
        scheduleNextPoll()
      }
    }, POLL_INTERVAL_MS)
  }, [fetchCount])

  const refresh = useCallback(() => {
    void fetchCount()
  }, [fetchCount])

  const incrementUnread = useCallback(() => {
    setUnreadCount(prev => prev + 1)
  }, [])

  const resetUnread = useCallback(() => {
    setUnreadCount(0)
  }, [])

  useEffect(() => {
    mountedRef.current = true

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void fetchCount()
        scheduleNextPoll()
      } else if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
    }

    document.addEventListener('visibilitychange', handleVisibilityChange)
    void fetchCount()
    scheduleNextPoll()

    return () => {
      mountedRef.current = false
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
    }
  }, [fetchCount, scheduleNextPoll])

  return { unreadCount, incrementUnread, resetUnread, refresh }
}
