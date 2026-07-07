/**
 * Unit Tests for useExecutionPolling Hook - Issue #737
 *
 * This test suite validates the useExecutionPolling hook functionality including:
 * - Polling at specified interval
 * - Stopping polling on terminal state
 * - Handling polling errors
 * - Cleanup on unmount (memory leak prevention)
 * - Enable/disable polling
 * - Callback execution
 *
 * Coverage target: >50%
 */

import { renderHook, act, waitFor } from '@testing-library/react'

import type { AgentExecution } from '../../src/services/agentService'

// Mock the agentService
const mockAgentService = {
  getExecutionStatus: jest.fn(),
}

jest.mock('../../src/services/agentService', () => ({
  agentService: mockAgentService,
}))

// Import the hook after mocking
import { useExecutionPolling } from '../../src/hooks/useExecutionPolling'

describe('useExecutionPolling', () => {
  const mockExecution: AgentExecution = {
    id: 'exec-123',
    agent_id: 'agent-456',
    user_id: 'user-789',
    status: 'running',
    input: { message: 'Hello' },
    error: null,
    started_at: '2023-01-01T00:00:00Z',
    ended_at: null,
    duration: null,
    version: 1,
  }

  // getExecutionStatus now returns the bare AgentExecution (no envelope).
  const createMockResponse = (execution: AgentExecution): AgentExecution =>
    execution

  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    mockAgentService.getExecutionStatus.mockResolvedValue(
      createMockResponse(mockExecution)
    )
  })

  afterEach(() => {
    jest.useRealTimers()
  })

  describe('Initialization', () => {
    it('should return initial state when no execution ID', () => {
      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: null,
        })
      )

      expect(result.current.execution).toBeNull()
      expect(result.current.isPolling).toBe(false)
      expect(result.current.error).toBeNull()
      expect(typeof result.current.stopPolling).toBe('function')
    })

    it('should not start polling when executionId is null', () => {
      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: null,
        })
      )

      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })

    it('should not start polling when enabled is false', () => {
      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          enabled: false,
        })
      )

      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })
  })

  describe('Polling Behavior', () => {
    it('should start polling when executionId is provided', async () => {
      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledWith(
          'test-team-id',
          'exec-123'
        )
      })
    })

    it('should poll at specified interval', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      // The result is used implicitly to verify hook behavior
      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          interval: 1000,
        })
      )

      // First call
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(1)
      })

      // Advance timer for next poll
      await act(async () => {
        jest.advanceTimersByTime(1000)
      })

      // Should poll again
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(2)
      })
    })

    it('should use default interval of 5000ms', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      // First call
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(1)
      })

      // Advance timer by 4999ms - should not poll yet
      await act(async () => {
        jest.advanceTimersByTime(4999)
      })

      expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(1)

      // Advance remaining 1ms
      await act(async () => {
        jest.advanceTimersByTime(1)
      })

      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(2)
      })
    })

    it('should update execution state on successful poll', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      await waitFor(() => {
        expect(result.current.execution).toEqual(mockExecution)
      })
    })

    it('should set isPolling to true while polling', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      await waitFor(() => {
        expect(result.current.isPolling).toBe(true)
      })
    })
  })

  describe('Terminal States', () => {
    const terminalStates = [
      'success',
      'error',
      'completed',
      'failed',
      'cancelled',
    ]

    terminalStates.forEach(status => {
      it(`should stop polling on ${status} status`, async () => {
        const terminalExecution: AgentExecution = {
          ...mockExecution,
          status: status as AgentExecution['status'],
        }

        mockAgentService.getExecutionStatus.mockResolvedValue(
          createMockResponse(terminalExecution)
        )

        const { result } = renderHook(() =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId: 'exec-123',
            interval: 1000,
          })
        )

        // Wait for first poll
        await waitFor(() => {
          expect(result.current.execution?.status).toBe(status)
        })

        // Should stop polling
        expect(result.current.isPolling).toBe(false)

        // Clear the mock call count
        mockAgentService.getExecutionStatus.mockClear()

        // Advance timer
        await act(async () => {
          jest.advanceTimersByTime(2000)
        })

        // Should not poll again
        expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
      })
    })

    it('should call onComplete callback on terminal state', async () => {
      const completedExecution: AgentExecution = {
        ...mockExecution,
        status: 'completed',
      }

      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(completedExecution)
      )

      const onComplete = jest.fn()

      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          onComplete,
        })
      )

      await waitFor(() => {
        expect(onComplete).toHaveBeenCalledWith(completedExecution)
      })
    })

    it('should call onComplete only once', async () => {
      const completedExecution: AgentExecution = {
        ...mockExecution,
        status: 'completed',
      }

      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(completedExecution)
      )

      const onComplete = jest.fn()

      const { rerender } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          onComplete,
        })
      )

      await waitFor(() => {
        expect(onComplete).toHaveBeenCalledTimes(1)
      })

      // Rerender should not trigger another call
      rerender()

      expect(onComplete).toHaveBeenCalledTimes(1)
    })
  })

  describe('Error Handling', () => {
    it('should set error state on polling failure', async () => {
      const errorMessage = 'Failed to fetch execution status'
      mockAgentService.getExecutionStatus.mockRejectedValue(
        new Error(errorMessage)
      )

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      await waitFor(() => {
        expect(result.current.error).toBeInstanceOf(Error)
        expect(result.current.error?.message).toBe(errorMessage)
      })
    })

    it('should stop polling on error', async () => {
      mockAgentService.getExecutionStatus.mockRejectedValue(
        new Error('Network error')
      )

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          interval: 1000,
        })
      )

      await waitFor(() => {
        expect(result.current.error).not.toBeNull()
      })

      expect(result.current.isPolling).toBe(false)

      // Clear the mock
      mockAgentService.getExecutionStatus.mockClear()

      // Advance timer
      await act(async () => {
        jest.advanceTimersByTime(2000)
      })

      // Should not poll again
      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })

    it('should call onError callback on polling error', async () => {
      const error = new Error('API Error')
      mockAgentService.getExecutionStatus.mockRejectedValue(error)

      const onError = jest.fn()

      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          onError,
        })
      )

      await waitFor(() => {
        expect(onError).toHaveBeenCalledWith(error)
      })
    })

    it('should wrap non-Error objects as Error', async () => {
      mockAgentService.getExecutionStatus.mockRejectedValue('String error')

      const onError = jest.fn()

      renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          onError,
        })
      )

      await waitFor(() => {
        expect(onError).toHaveBeenCalledWith(expect.any(Error))
        expect(onError.mock.calls[0][0].message).toBe('Failed to poll status')
      })
    })

    it('should clear error on successful poll after error', async () => {
      // First poll fails
      mockAgentService.getExecutionStatus.mockRejectedValueOnce(
        new Error('Temporary error')
      )

      const { result, rerender } = renderHook(
        ({ enabled }) =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId: 'exec-123',
            enabled,
          }),
        { initialProps: { enabled: true } }
      )

      await waitFor(() => {
        expect(result.current.error).not.toBeNull()
      })

      // Disable polling
      rerender({ enabled: false })

      // Setup successful response
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      // Re-enable polling
      rerender({ enabled: true })

      await waitFor(() => {
        expect(result.current.error).toBeNull()
        expect(result.current.execution).toEqual(mockExecution)
      })
    })
  })

  describe('Stop Polling', () => {
    it('should stop polling when stopPolling is called', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          interval: 1000,
        })
      )

      // Wait for first poll
      await waitFor(() => {
        expect(result.current.isPolling).toBe(true)
      })

      // Stop polling
      act(() => {
        result.current.stopPolling()
      })

      expect(result.current.isPolling).toBe(false)

      // Clear mock
      mockAgentService.getExecutionStatus.mockClear()

      // Advance timer
      await act(async () => {
        jest.advanceTimersByTime(2000)
      })

      // Should not poll again
      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })

    it('should be safe to call stopPolling multiple times', () => {
      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: null,
        })
      )

      // Should not throw
      expect(() => {
        result.current.stopPolling()
        result.current.stopPolling()
        result.current.stopPolling()
      }).not.toThrow()
    })
  })

  describe('Cleanup on Unmount', () => {
    it('should clear timeout on unmount', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { unmount } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
          interval: 1000,
        })
      )

      // Wait for first poll
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(1)
      })

      // Unmount
      unmount()

      // Clear mock
      mockAgentService.getExecutionStatus.mockClear()

      // Advance timer
      await act(async () => {
        jest.advanceTimersByTime(2000)
      })

      // Should not poll after unmount - prevents memory leaks
      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })

    it('should not cause memory leaks with rapid enable/disable', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { rerender } = renderHook(
        ({ enabled }) =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId: 'exec-123',
            enabled,
            interval: 100,
          }),
        { initialProps: { enabled: true } }
      )

      // Rapidly toggle enabled
      for (let i = 0; i < 10; i++) {
        rerender({ enabled: false })
        rerender({ enabled: true })
      }

      // Clear mock
      mockAgentService.getExecutionStatus.mockClear()

      // Advance timer
      await act(async () => {
        jest.advanceTimersByTime(500)
      })

      // Should have a reasonable number of calls (not 10x)
      expect(
        mockAgentService.getExecutionStatus.mock.calls.length
      ).toBeLessThanOrEqual(5)
    })
  })

  describe('Execution ID Changes', () => {
    it('should restart polling when executionId changes', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { rerender } = renderHook(
        ({ executionId }) =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId,
          }),
        { initialProps: { executionId: 'exec-123' as string | null } }
      )

      // Wait for first poll
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledWith(
          'test-team-id',
          'exec-123'
        )
      })

      // Change execution ID
      rerender({ executionId: 'exec-456' })

      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledWith(
          'test-team-id',
          'exec-456'
        )
      })
    })

    it('should stop polling when executionId becomes null', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(
        createMockResponse(mockExecution)
      )

      const { result, rerender } = renderHook(
        ({ executionId }) =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId,
            interval: 1000,
          }),
        { initialProps: { executionId: 'exec-123' as string | null } }
      )

      // Wait for first poll
      await waitFor(() => {
        expect(result.current.isPolling).toBe(true)
      })

      // Set executionId to null
      rerender({ executionId: null })

      // Clear mock
      mockAgentService.getExecutionStatus.mockClear()

      // Advance timer
      await act(async () => {
        jest.advanceTimersByTime(2000)
      })

      // Should not poll
      expect(mockAgentService.getExecutionStatus).not.toHaveBeenCalled()
    })
  })

  describe('Callback Reference Updates', () => {
    it('should handle onComplete callback updates', async () => {
      const completedExecution: AgentExecution = {
        ...mockExecution,
        status: 'completed',
      }

      // Return running first, then completed
      mockAgentService.getExecutionStatus
        .mockResolvedValueOnce(createMockResponse(mockExecution))
        .mockResolvedValueOnce(createMockResponse(completedExecution))

      const onComplete1 = jest.fn()
      const onComplete2 = jest.fn()

      const { rerender } = renderHook(
        ({ onComplete }) =>
          useExecutionPolling({
            teamId: 'test-team-id',
            executionId: 'exec-123',
            interval: 1000,
            onComplete,
          }),
        { initialProps: { onComplete: onComplete1 } }
      )

      // Wait for first poll
      await waitFor(() => {
        expect(mockAgentService.getExecutionStatus).toHaveBeenCalledTimes(1)
      })

      // Change callback before completion
      rerender({ onComplete: onComplete2 })

      // Advance timer for next poll
      await act(async () => {
        jest.advanceTimersByTime(1000)
      })

      await waitFor(() => {
        expect(onComplete2).toHaveBeenCalledWith(completedExecution)
      })

      // Old callback should not have been called
      expect(onComplete1).not.toHaveBeenCalled()
    })
  })

  describe('Response Data Handling', () => {
    it('should handle unwrapped response data', async () => {
      mockAgentService.getExecutionStatus.mockResolvedValue(mockExecution)

      const { result } = renderHook(() =>
        useExecutionPolling({
          teamId: 'test-team-id',
          executionId: 'exec-123',
        })
      )

      await waitFor(() => {
        expect(result.current.execution).toEqual(mockExecution)
      })
    })
  })
})
