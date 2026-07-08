import { renderHook } from '@testing-library/react'
import { useAlerts } from '../../src/hooks/useAlerts'
import { useAlertContext } from '../../src/contexts/AlertContext'
import type { AlertContextValue, Alert } from '../../src/types/alert'

// Mock the AlertContext
jest.mock('../../src/contexts/AlertContext')

const mockUseAlertContext = useAlertContext as jest.MockedFunction<
  typeof useAlertContext
>

describe('useAlerts', () => {
  let mockAlertContextValue: AlertContextValue
  let mockAlerts: Alert[]
  let mockShowAlert: jest.MockedFunction<
    (options: { message: string; title?: string; type: string }) => string
  >
  let mockDismissAlert: jest.MockedFunction<(id: string) => void>
  let mockClearAll: jest.MockedFunction<() => void>

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks()

    // Create mock alerts
    mockAlerts = [
      {
        id: 'alert-1',
        message: 'Success message',
        type: 'success',
        title: 'Success',
        duration: 5000,
        persistent: false,
        createdAt: Date.now(),
      },
      {
        id: 'alert-2',
        message: 'Error message',
        type: 'error',
        duration: 8000,
        persistent: false,
        createdAt: Date.now(),
      },
    ]

    // Create mock functions
    mockShowAlert = jest.fn().mockReturnValue('mock-alert-id')
    mockDismissAlert = jest.fn()
    mockClearAll = jest.fn()

    // Create mock context value
    mockAlertContextValue = {
      alerts: mockAlerts,
      showAlert: mockShowAlert,
      dismissAlert: mockDismissAlert,
      clearAll: mockClearAll,
    }

    // Setup mock implementation
    mockUseAlertContext.mockReturnValue(mockAlertContextValue)
  })

  describe('Hook Interface', () => {
    it('should return all expected properties and functions', () => {
      const { result } = renderHook(() => useAlerts())

      expect(result.current).toHaveProperty('alerts')
      expect(result.current).toHaveProperty('showAlert')
      expect(result.current).toHaveProperty('showSuccess')
      expect(result.current).toHaveProperty('showError')
      expect(result.current).toHaveProperty('showWarning')
      expect(result.current).toHaveProperty('showInfo')
      expect(result.current).toHaveProperty('dismissAlert')
      expect(result.current).toHaveProperty('clearAll')
    })

    it('should expose alerts from context', () => {
      const { result } = renderHook(() => useAlerts())

      expect(result.current.alerts).toBe(mockAlerts)
      expect(result.current.alerts).toHaveLength(2)
    })

    it('should expose showAlert function from context', () => {
      const { result } = renderHook(() => useAlerts())

      expect(result.current.showAlert).toBe(mockShowAlert)
    })

    it('should expose dismissAlert function from context', () => {
      const { result } = renderHook(() => useAlerts())

      expect(result.current.dismissAlert).toBe(mockDismissAlert)
    })

    it('should expose clearAll function from context', () => {
      const { result } = renderHook(() => useAlerts())

      expect(result.current.clearAll).toBe(mockClearAll)
    })
  })

  describe('Alert Operations', () => {
    describe('showSuccess', () => {
      it('should call showAlert with success type and message', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showSuccess('Success message')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Success message',
          title: undefined,
          type: 'success',
        })
        expect(alertId).toBe('mock-alert-id')
      })

      it('should call showAlert with success type, message, and title', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.showSuccess('Success message', 'Success Title')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Success message',
          title: 'Success Title',
          type: 'success',
        })
      })

      it('should return the alert ID from context', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showSuccess('Success message')

        expect(alertId).toBe('mock-alert-id')
      })
    })

    describe('showError', () => {
      it('should call showAlert with error type and message', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showError('Error message')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Error message',
          title: undefined,
          type: 'error',
        })
        expect(alertId).toBe('mock-alert-id')
      })

      it('should call showAlert with error type, message, and title', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.showError('Error message', 'Error Title')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Error message',
          title: 'Error Title',
          type: 'error',
        })
      })

      it('should return the alert ID from context', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showError('Error message')

        expect(alertId).toBe('mock-alert-id')
      })
    })

    describe('showWarning', () => {
      it('should call showAlert with warning type and message', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showWarning('Warning message')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Warning message',
          title: undefined,
          type: 'warning',
        })
        expect(alertId).toBe('mock-alert-id')
      })

      it('should call showAlert with warning type, message, and title', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.showWarning('Warning message', 'Warning Title')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Warning message',
          title: 'Warning Title',
          type: 'warning',
        })
      })

      it('should return the alert ID from context', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showWarning('Warning message')

        expect(alertId).toBe('mock-alert-id')
      })
    })

    describe('showInfo', () => {
      it('should call showAlert with info type and message', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showInfo('Info message')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Info message',
          title: undefined,
          type: 'info',
        })
        expect(alertId).toBe('mock-alert-id')
      })

      it('should call showAlert with info type, message, and title', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.showInfo('Info message', 'Info Title')

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Info message',
          title: 'Info Title',
          type: 'info',
        })
      })

      it('should return the alert ID from context', () => {
        const { result } = renderHook(() => useAlerts())

        const alertId = result.current.showInfo('Info message')

        expect(alertId).toBe('mock-alert-id')
      })
    })

    describe('dismissAlert', () => {
      it('should call dismissAlert from context with provided ID', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.dismissAlert('alert-id-123')

        expect(mockDismissAlert).toHaveBeenCalledWith('alert-id-123')
      })
    })

    describe('clearAll', () => {
      it('should call clearAll from context', () => {
        const { result } = renderHook(() => useAlerts())

        result.current.clearAll()

        expect(mockClearAll).toHaveBeenCalledWith()
      })
    })
  })

  describe('Hook Behavior', () => {
    it('should consume AlertContext properly', () => {
      renderHook(() => useAlerts())

      expect(mockUseAlertContext).toHaveBeenCalledTimes(1)
    })

    it('should handle context updates', () => {
      const { result, rerender } = renderHook(() => useAlerts())

      expect(result.current.alerts).toBe(mockAlerts)

      // Update context with new alerts
      const newAlerts = [
        {
          id: 'alert-3',
          message: 'New alert',
          type: 'info' as const,
          duration: 4000,
          persistent: false,
          createdAt: Date.now(),
        },
      ]

      mockAlertContextValue.alerts = newAlerts
      mockUseAlertContext.mockReturnValue(mockAlertContextValue)

      rerender()

      expect(result.current.alerts).toBe(newAlerts)
    })

    describe('Function Stability', () => {
      it('should maintain stable function references across re-renders', () => {
        const { result, rerender } = renderHook(() => useAlerts())

        const firstRenderFunctions = {
          showSuccess: result.current.showSuccess,
          showError: result.current.showError,
          showWarning: result.current.showWarning,
          showInfo: result.current.showInfo,
        }

        rerender()

        expect(result.current.showSuccess).toBe(
          firstRenderFunctions.showSuccess
        )
        expect(result.current.showError).toBe(firstRenderFunctions.showError)
        expect(result.current.showWarning).toBe(
          firstRenderFunctions.showWarning
        )
        expect(result.current.showInfo).toBe(firstRenderFunctions.showInfo)
      })

      it('should maintain stable function references when showAlert changes', () => {
        const { result, rerender } = renderHook(() => useAlerts())

        const firstRenderFunctions = {
          showSuccess: result.current.showSuccess,
          showError: result.current.showError,
          showWarning: result.current.showWarning,
          showInfo: result.current.showInfo,
        }

        // Update showAlert function reference
        const newShowAlert = jest.fn().mockReturnValue('new-alert-id')
        mockAlertContextValue.showAlert = newShowAlert
        mockUseAlertContext.mockReturnValue(mockAlertContextValue)

        rerender()

        // Functions should be re-created due to dependency change
        expect(result.current.showSuccess).not.toBe(
          firstRenderFunctions.showSuccess
        )
        expect(result.current.showError).not.toBe(
          firstRenderFunctions.showError
        )
        expect(result.current.showWarning).not.toBe(
          firstRenderFunctions.showWarning
        )
        expect(result.current.showInfo).not.toBe(firstRenderFunctions.showInfo)

        // But they should still work correctly
        result.current.showSuccess('Test message')
        expect(newShowAlert).toHaveBeenCalledWith({
          message: 'Test message',
          title: undefined,
          type: 'success',
        })
      })
    })

    describe('Error Handling', () => {
      it('should propagate context errors', () => {
        mockUseAlertContext.mockImplementation(() => {
          throw new Error(
            'useAlertContext must be used within an AlertProvider'
          )
        })

        expect(() => {
          renderHook(() => useAlerts())
        }).toThrow('useAlertContext must be used within an AlertProvider')
      })
    })
  })

  describe('Hook Integration', () => {
    it('should work correctly with all alert types in sequence', () => {
      const { result } = renderHook(() => useAlerts())

      // Test all convenience methods
      result.current.showSuccess('Success')
      result.current.showError('Error')
      result.current.showWarning('Warning')
      result.current.showInfo('Info')

      expect(mockShowAlert).toHaveBeenCalledTimes(4)
      expect(mockShowAlert).toHaveBeenNthCalledWith(1, {
        message: 'Success',
        title: undefined,
        type: 'success',
      })
      expect(mockShowAlert).toHaveBeenNthCalledWith(2, {
        message: 'Error',
        title: undefined,
        type: 'error',
      })
      expect(mockShowAlert).toHaveBeenNthCalledWith(3, {
        message: 'Warning',
        title: undefined,
        type: 'warning',
      })
      expect(mockShowAlert).toHaveBeenNthCalledWith(4, {
        message: 'Info',
        title: undefined,
        type: 'info',
      })
    })

    it('should handle complex alert management workflow', () => {
      const { result } = renderHook(() => useAlerts())

      // Show multiple alerts
      const successId = result.current.showSuccess('Operation successful')
      result.current.showError('Operation failed')

      // Dismiss specific alert
      result.current.dismissAlert(successId)

      // Clear all alerts
      result.current.clearAll()

      expect(mockShowAlert).toHaveBeenCalledTimes(2)
      expect(mockDismissAlert).toHaveBeenCalledWith(successId)
      expect(mockClearAll).toHaveBeenCalledTimes(1)
    })

    it('should handle empty alerts array', () => {
      mockAlertContextValue.alerts = []
      mockUseAlertContext.mockReturnValue(mockAlertContextValue)

      const { result } = renderHook(() => useAlerts())

      expect(result.current.alerts).toEqual([])
      expect(result.current.alerts).toHaveLength(0)
    })

    it('should handle context with undefined values gracefully', () => {
      mockAlertContextValue.alerts = []
      mockUseAlertContext.mockReturnValue(mockAlertContextValue)

      const { result } = renderHook(() => useAlerts())

      // Should not throw when calling functions
      expect(() => {
        result.current.showSuccess('Test')
        result.current.dismissAlert('test-id')
        result.current.clearAll()
      }).not.toThrow()
    })
  })

  describe('Performance Optimization', () => {
    it('should optimize re-renders by memoizing convenience functions', () => {
      let renderCount = 0
      const TestComponent = () => {
        renderCount++
        const { showSuccess } = useAlerts()
        return showSuccess
      }

      const { result, rerender } = renderHook(() => TestComponent())

      const initialFunction = result.current
      const initialRenderCount = renderCount

      // Re-render without changing context
      rerender()

      expect(result.current).toBe(initialFunction)
      expect(renderCount).toBe(initialRenderCount + 1)
    })
  })
})
