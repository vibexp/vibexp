import { renderHook, waitFor } from '@testing-library/react'
import {
  useEventPolling,
  EVENT_POLLING_INTERVAL_MS,
} from '../../src/hooks/useEventPolling'

// authService no longer used by useEventPolling (cookie-based auth)

// Mock fetch
global.fetch = jest.fn()

describe('useEventPolling', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
  })

  afterEach(() => {
    jest.useRealTimers()
  })

  it('should start polling when enabled with execution ID', async () => {
    const mockResponse = {
      execution_id: 'exec-1',
      status: 'pending',
      current_state: 'working',
      events: [],
      has_more: true,
      next_sequence: 0,
    }

    ;(global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: async () => mockResponse,
    })

    const { result } = renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
      })
    )

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/agents/executions/exec-1/events?since=0'),
        expect.objectContaining({
          credentials: 'include',
        })
      )
    })

    expect(result.current.isPolling).toBe(true)
  })

  it('should accumulate events from multiple polls', async () => {
    const firstResponse = {
      execution_id: 'exec-1',
      status: 'pending',
      current_state: 'working',
      events: [
        {
          id: 'event-1',
          execution_id: 'exec-1',
          event_type: 'status-update',
          event_data: { state: 'working' },
          sequence_number: 1,
          received_at: new Date().toISOString(),
        },
      ],
      has_more: true,
      next_sequence: 2,
    }

    const secondResponse = {
      execution_id: 'exec-1',
      status: 'pending',
      current_state: 'working',
      events: [
        {
          id: 'event-2',
          execution_id: 'exec-1',
          event_type: 'artifact-update',
          event_data: { artifactId: 'art-1', append: false },
          sequence_number: 2,
          received_at: new Date().toISOString(),
        },
      ],
      has_more: true,
      next_sequence: 3,
    }

    ;(global.fetch as jest.Mock)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => firstResponse,
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => secondResponse,
      })

    const { result } = renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
      })
    )

    // Wait for first poll
    await waitFor(() => {
      expect(result.current.events).toHaveLength(1)
    })

    // Advance time to trigger second poll
    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS)

    // Wait for second poll
    await waitFor(() => {
      expect(result.current.events).toHaveLength(2)
    })

    expect(result.current.events[0].sequence_number).toBe(1)
    expect(result.current.events[1].sequence_number).toBe(2)
  })

  it('should advance cursor with each poll', async () => {
    const responses = [
      {
        execution_id: 'exec-1',
        status: 'pending',
        events: [
          {
            id: 'e1',
            sequence_number: 1,
            event_type: 'test',
            event_data: {},
            execution_id: 'exec-1',
            received_at: '',
          },
        ],
        has_more: true,
        next_sequence: 2,
        current_state: 'working',
      },
      {
        execution_id: 'exec-1',
        status: 'pending',
        events: [
          {
            id: 'e2',
            sequence_number: 2,
            event_type: 'test',
            event_data: {},
            execution_id: 'exec-1',
            received_at: '',
          },
        ],
        has_more: true,
        next_sequence: 3,
        current_state: 'working',
      },
    ]

    ;(global.fetch as jest.Mock)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => responses[0],
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => responses[1],
      })

    // Use real timers
    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
        pollingInterval: 500, // Faster for testing
      })
    )

    // First poll should use since=0
    await waitFor(
      () => {
        expect(global.fetch).toHaveBeenCalledWith(
          expect.stringContaining('since=0'),
          expect.any(Object)
        )
      },
      { timeout: 3000 }
    )

    // Wait for second poll (500ms + buffer)
    await waitFor(
      () => {
        expect(global.fetch).toHaveBeenCalledWith(
          expect.stringContaining('since=2'),
          expect.any(Object)
        )
      },
      { timeout: 2000 }
    )
  })

  it('should stop polling when status is complete', async () => {
    const mockResponse = {
      execution_id: 'exec-1',
      status: 'success',
      current_state: 'completed',
      events: [],
      has_more: false,
      next_sequence: 5,
    }

    ;(global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: async () => mockResponse,
    })

    const onComplete = jest.fn()

    // Use real timers
    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    const { result } = renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
        onComplete,
      })
    )

    // Wait for state to update
    await waitFor(
      () => {
        expect(result.current.status).toBe('success')
      },
      { timeout: 5000 }
    )

    // Verify polling stopped and callback was called
    expect(result.current.isPolling).toBe(false)
    expect(onComplete).toHaveBeenCalledWith('success')
  })

  it('should call onEvent callback for each new event', async () => {
    const mockResponse = {
      execution_id: 'exec-1',
      status: 'pending',
      current_state: 'working',
      events: [
        {
          id: 'event-1',
          execution_id: 'exec-1',
          event_type: 'artifact-update',
          event_data: { test: 'data' },
          sequence_number: 1,
          received_at: new Date().toISOString(),
        },
      ],
      has_more: true,
      next_sequence: 2,
    }

    ;(global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: async () => mockResponse,
    })

    const onEvent = jest.fn()

    renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
        onEvent,
      })
    )

    await waitFor(() => {
      expect(onEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'event-1',
          event_type: 'artifact-update',
        })
      )
    })
  })

  it('should not poll when disabled', async () => {
    renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: false,
      })
    )

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS * 2)

    expect(global.fetch).not.toHaveBeenCalled()
  })

  it('should not poll without execution ID', async () => {
    renderHook(() =>
      useEventPolling({
        executionId: null,
        enabled: true,
      })
    )

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS * 2)

    expect(global.fetch).not.toHaveBeenCalled()
  })

  it('should handle fetch errors gracefully', async () => {
    ;(global.fetch as jest.Mock).mockRejectedValue(new Error('Network error'))

    // Use real timers
    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    const { result } = renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
      })
    )

    // Wait for first error
    await waitFor(
      () => {
        expect(result.current.error).toBe('Network error')
      },
      { timeout: 5000 }
    )

    // First call should have happened
    expect(global.fetch).toHaveBeenCalledTimes(1)

    // Wait for retry (with backoff: interval * 2 = 6 seconds)
    await waitFor(
      () => {
        expect(global.fetch).toHaveBeenCalledTimes(2)
      },
      { timeout: 10000 }
    )
  }, 15000)

  it('should respect custom polling interval', async () => {
    const customInterval = 1000 // 1 second for faster test

    const mockResponse = {
      execution_id: 'exec-1',
      status: 'pending',
      events: [],
      has_more: true,
      next_sequence: 0,
      current_state: 'working',
    }

    ;(global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: async () => mockResponse,
    })

    // Use real timers
    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    renderHook(() =>
      useEventPolling({
        executionId: 'exec-1',
        enabled: true,
        pollingInterval: customInterval,
      })
    )

    // Wait for first poll
    await waitFor(
      () => {
        expect(global.fetch).toHaveBeenCalledTimes(1)
      },
      { timeout: 3000 }
    )

    // Wait for second poll (custom interval + buffer)
    await waitFor(
      () => {
        expect(global.fetch).toHaveBeenCalledTimes(2)
      },
      { timeout: 3000 }
    )
  })
})
