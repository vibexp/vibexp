import { renderHook, waitFor } from '@testing-library/react'

// Mock the generated client; unwrap stays real so the hook exercises the same
// success/error resolution production uses (see searchService.test.ts). The
// module under test is imported AFTER the mock so the factory runs once the
// mock object is initialised.
const mockGeneratedClient = {
  GET: jest.fn(),
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

import {
  useEventPolling,
  EVENT_POLLING_INTERVAL_MS,
} from '../../src/hooks/useEventPolling'
import { ApiError } from '../../src/types/errors'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const TEAM_ID = 'team-1'
const EXECUTION_ID = 'exec-1'
const EVENTS_PATH = '/api/v1/{team_id}/agents/executions/{id}/events'

// The `since` query value from the Nth (1-based) GET call.
function sinceOf(callIndex: number): number | undefined {
  const call = mockGeneratedClient.GET.mock.calls[callIndex - 1] as
    | [string, { params: { query: { since?: number } } }]
    | undefined
  return call?.[1].params.query.since
}

describe('useEventPolling', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
  })

  afterEach(() => {
    jest.useRealTimers()
  })

  it('should start polling when enabled with team ID and execution ID', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [],
        has_more: true,
        next_sequence: 0,
      })
    )

    const { result } = renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
      })
    )

    await waitFor(() => {
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        EVENTS_PATH,
        expect.objectContaining({
          params: {
            path: { team_id: TEAM_ID, id: EXECUTION_ID },
            query: { since: 0 },
          },
        })
      )
    })

    expect(result.current.isPolling).toBe(true)
  })

  it('should accumulate events from multiple polls', async () => {
    mockGeneratedClient.GET.mockReturnValueOnce(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [
          {
            id: 'event-1',
            execution_id: EXECUTION_ID,
            event_type: 'status-update',
            event_data: { state: 'working' },
            sequence_number: 1,
            received_at: new Date().toISOString(),
          },
        ],
        has_more: true,
        next_sequence: 2,
      })
    ).mockReturnValueOnce(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [
          {
            id: 'event-2',
            execution_id: EXECUTION_ID,
            event_type: 'artifact-update',
            event_data: { artifactId: 'art-1', append: false },
            sequence_number: 2,
            received_at: new Date().toISOString(),
          },
        ],
        has_more: true,
        next_sequence: 3,
      })
    )

    const { result } = renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
      })
    )

    await waitFor(() => {
      expect(result.current.events).toHaveLength(1)
    })

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS)

    await waitFor(() => {
      expect(result.current.events).toHaveLength(2)
    })

    expect(result.current.events[0].sequence_number).toBe(1)
    expect(result.current.events[1].sequence_number).toBe(2)
  })

  it('should advance the cursor with each poll', async () => {
    mockGeneratedClient.GET.mockReturnValueOnce(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [
          {
            id: 'e1',
            execution_id: EXECUTION_ID,
            event_type: 'test',
            event_data: {},
            sequence_number: 1,
            received_at: '',
          },
        ],
        has_more: true,
        next_sequence: 2,
      })
    ).mockReturnValue(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [
          {
            id: 'e2',
            execution_id: EXECUTION_ID,
            event_type: 'test',
            event_data: {},
            sequence_number: 2,
            received_at: '',
          },
        ],
        has_more: true,
        next_sequence: 3,
      })
    )

    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
        pollingInterval: 500,
      })
    )

    // First poll uses since=0.
    await waitFor(
      () => {
        expect(sinceOf(1)).toBe(0)
      },
      { timeout: 3000 }
    )

    // Second poll advances to the last returned sequence.
    await waitFor(
      () => {
        expect(sinceOf(2)).toBe(2)
      },
      { timeout: 2000 }
    )
  })

  it('should stop polling when status is complete', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        execution_id: EXECUTION_ID,
        status: 'success',
        current_state: 'completed',
        events: [],
        has_more: false,
        next_sequence: 5,
      })
    )

    const onComplete = jest.fn()

    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    const { result } = renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
        onComplete,
      })
    )

    await waitFor(
      () => {
        expect(result.current.status).toBe('success')
      },
      { timeout: 5000 }
    )

    expect(result.current.isPolling).toBe(false)
    expect(onComplete).toHaveBeenCalledWith('success')
  })

  it('should call onEvent callback for each new event', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [
          {
            id: 'event-1',
            execution_id: EXECUTION_ID,
            event_type: 'artifact-update',
            event_data: { test: 'data' },
            sequence_number: 1,
            received_at: new Date().toISOString(),
          },
        ],
        has_more: true,
        next_sequence: 2,
      })
    )

    const onEvent = jest.fn()

    renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
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

  it('should not poll when disabled', () => {
    renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: false,
      })
    )

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS * 2)

    expect(mockGeneratedClient.GET).not.toHaveBeenCalled()
  })

  it('should not poll without an execution ID', () => {
    renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: null,
        enabled: true,
      })
    )

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS * 2)

    expect(mockGeneratedClient.GET).not.toHaveBeenCalled()
  })

  it('should not poll without a team ID', () => {
    renderHook(() =>
      useEventPolling({
        teamId: undefined,
        executionId: EXECUTION_ID,
        enabled: true,
      })
    )

    jest.advanceTimersByTime(EVENT_POLLING_INTERVAL_MS * 2)

    expect(mockGeneratedClient.GET).not.toHaveBeenCalled()
  })

  it('should handle fetch errors gracefully and retry', async () => {
    mockGeneratedClient.GET.mockRejectedValue(new Error('Network error'))

    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    const { result } = renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
      })
    )

    await waitFor(
      () => {
        expect(result.current.error).toBe('Network error')
      },
      { timeout: 5000 }
    )

    expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(1)

    // Retry after backoff (interval * 2 = 6 seconds).
    await waitFor(
      () => {
        expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(2)
      },
      { timeout: 10000 }
    )
  }, 15000)

  it('should stop without surfacing an error or retrying on a 401', async () => {
    // On a 401 the hook redirects to /sign-in (via window.location) and returns
    // before the error/backoff path — so, unlike a generic error, it neither
    // surfaces an error nor schedules a retry. We assert those observable effects
    // (jsdom cannot intercept the location assignment, which is a harmless no-op).
    mockGeneratedClient.GET.mockRejectedValue(
      new ApiError({
        type: 'https://api.vibexp.io/errors/unauthorized',
        title: 'Unauthorized',
        status: 401,
        detail: 'Session expired',
        code: 'UNAUTHENTICATED',
        request_id: 'req-1',
        timestamp: '2026-01-01T00:00:00Z',
      })
    )

    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    const { result } = renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
        pollingInterval: 50,
      })
    )

    await waitFor(
      () => {
        expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(1)
      },
      { timeout: 5000 }
    )
    await waitFor(() => expect(result.current.isPolling).toBe(false), {
      timeout: 5000,
    })
    // The 401 branch returns before setError; a generic error would set one.
    expect(result.current.error).toBeNull()
    // And it schedules no backoff retry (interval*2 = 100ms) — a generic error would.
    await new Promise(resolve => setTimeout(resolve, 250))
    expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(1)
  }, 10000)

  it('should respect a custom polling interval', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        execution_id: EXECUTION_ID,
        status: 'pending',
        current_state: 'working',
        events: [],
        has_more: true,
        next_sequence: 0,
      })
    )

    jest.runOnlyPendingTimers()
    jest.useRealTimers()

    renderHook(() =>
      useEventPolling({
        teamId: TEAM_ID,
        executionId: EXECUTION_ID,
        enabled: true,
        pollingInterval: 1000,
      })
    )

    await waitFor(
      () => {
        expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(1)
      },
      { timeout: 3000 }
    )

    await waitFor(
      () => {
        expect(mockGeneratedClient.GET).toHaveBeenCalledTimes(2)
      },
      { timeout: 3000 }
    )
  })
})
