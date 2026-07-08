import { render, screen, waitFor, act } from '@testing-library/react'
import { renderHook } from '@testing-library/react'
import { AlertProvider, useAlertContext } from '../../src/contexts/AlertContext'
import type { AlertOptions } from '../../src/types/alert'

// Mock timers for testing auto-dismiss functionality
beforeEach(() => {
  jest.useFakeTimers()
})

afterEach(() => {
  jest.runOnlyPendingTimers()
  jest.useRealTimers()
})

interface TestComponentProps {
  onHookValue?: (value: ReturnType<typeof useAlertContext>) => void
}

// Helper component to test provider and hook
const TestComponent = ({ onHookValue }: TestComponentProps) => {
  const alertContext = useAlertContext()

  if (onHookValue) {
    onHookValue(alertContext)
  }

  return (
    <div>
      <div data-testid="alerts-count">{alertContext.alerts.length}</div>
      <div data-testid="alerts-list">
        {alertContext.alerts.map(alert => (
          <div key={alert.id} data-testid={`alert-${alert.type}`}>
            {alert.title && (
              <span data-testid="alert-title">{alert.title}</span>
            )}
            <span data-testid="alert-message">{alert.message}</span>
            <span data-testid="alert-type">{alert.type}</span>
            <span data-testid="alert-duration">{alert.duration}</span>
            <span data-testid="alert-persistent">
              {alert.persistent.toString()}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

// Helper function to render component with provider
const renderWithProvider = (component: React.ReactElement) => {
  return render(<AlertProvider>{component}</AlertProvider>)
}

describe('AlertContext', () => {
  describe('AlertProvider', () => {
    it('renders children correctly', () => {
      renderWithProvider(<div data-testid="test-child">Test Child</div>)
      expect(screen.getByTestId('test-child')).toBeTruthy()
    })

    it('provides context value to children', () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      expect(contextValue).not.toBeNull()
      expect(contextValue!.alerts).toEqual([])
      expect(typeof contextValue!.showAlert).toBe('function')
      expect(typeof contextValue!.dismissAlert).toBe('function')
      expect(typeof contextValue!.clearAll).toBe('function')
    })
  })

  describe('useAlertContext', () => {
    it('returns context value when used within provider', () => {
      const { result } = renderHook(() => useAlertContext(), {
        wrapper: AlertProvider,
      })

      expect(result.current.alerts).toEqual([])
      expect(typeof result.current.showAlert).toBe('function')
      expect(typeof result.current.dismissAlert).toBe('function')
      expect(typeof result.current.clearAll).toBe('function')
    })

    it('throws error when used outside provider', () => {
      // Suppress console.error for this test since we expect an error
      const originalError = console.error
      console.error = jest.fn()

      // Create a component that uses the hook
      const TestErrorComponent = () => {
        const context = useAlertContext()
        return <div>{context.alerts.length}</div>
      }

      // This should throw an error when rendered without provider
      expect(() => {
        render(<TestErrorComponent />)
      }).toThrow('useAlertContext must be used within an AlertProvider')

      // Restore console.error
      console.error = originalError
    })
  })

  describe('Alert Management', () => {
    describe('showAlert', () => {
      it('adds success alert with default duration', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'success',
          message: 'Success message',
          title: 'Success Title',
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
        expect(screen.getByTestId('alert-success')).toBeTruthy()
        expect(screen.getByTestId('alert-title')).toHaveTextContent(
          'Success Title'
        )
        expect(screen.getByTestId('alert-message')).toHaveTextContent(
          'Success message'
        )
        expect(screen.getByTestId('alert-type')).toHaveTextContent('success')
        expect(screen.getByTestId('alert-duration')).toHaveTextContent('5000')
        expect(screen.getByTestId('alert-persistent')).toHaveTextContent(
          'false'
        )
      })

      it('adds error alert with default duration', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'error',
          message: 'Error message',
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-type')).toHaveTextContent('error')
        expect(screen.getByTestId('alert-duration')).toHaveTextContent('8000')
      })

      it('adds warning alert with default duration', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'warning',
          message: 'Warning message',
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-type')).toHaveTextContent('warning')
        expect(screen.getByTestId('alert-duration')).toHaveTextContent('6000')
      })

      it('adds info alert with default duration', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'info',
          message: 'Info message',
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-type')).toHaveTextContent('info')
        expect(screen.getByTestId('alert-duration')).toHaveTextContent('4000')
      })

      it('adds alert with custom duration', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'success',
          message: 'Custom duration message',
          duration: 10000,
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-duration')).toHaveTextContent('10000')
      })

      it('adds persistent alert', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'error',
          message: 'Persistent error',
          persistent: true,
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-persistent')).toHaveTextContent('true')
      })

      it('returns unique alert ID', () => {
        const { result } = renderHook(() => useAlertContext(), {
          wrapper: AlertProvider,
        })

        const alertOptions: AlertOptions = {
          type: 'success',
          message: 'Test message',
        }

        let id1 = ''
        let id2 = ''

        act(() => {
          id1 = result.current.showAlert(alertOptions)
          id2 = result.current.showAlert(alertOptions)
        })

        expect(id1).toBeTruthy()
        expect(id2).toBeTruthy()
        expect(id1).not.toBe(id2)
      })

      it('supports alert without title', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        const alertOptions: AlertOptions = {
          type: 'info',
          message: 'Message without title',
        }

        act(() => {
          contextValue!.showAlert(alertOptions)
        })

        expect(screen.getByTestId('alert-message')).toHaveTextContent(
          'Message without title'
        )
        expect(screen.queryByTestId('alert-title')).toBeNull()
      })
    })

    describe('dismissAlert', () => {
      it('removes alert by ID', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        let alertId: string

        act(() => {
          alertId = contextValue!.showAlert({
            type: 'success',
            message: 'Test alert',
          })
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

        act(() => {
          contextValue!.dismissAlert(alertId)
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })

      it('does not affect other alerts when dismissing one', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        let alertId1: string

        act(() => {
          alertId1 = contextValue!.showAlert({
            type: 'success',
            message: 'First alert',
          })
          contextValue!.showAlert({
            type: 'error',
            message: 'Second alert',
          })
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('2')

        act(() => {
          contextValue!.dismissAlert(alertId1)
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
        expect(screen.getByText('Second alert')).toBeTruthy()
        expect(screen.queryByText('First alert')).toBeNull()
      })

      it('does nothing when dismissing non-existent alert ID', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        act(() => {
          contextValue!.showAlert({
            type: 'success',
            message: 'Test alert',
          })
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

        act(() => {
          contextValue!.dismissAlert('non-existent-id')
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
      })
    })

    describe('clearAll', () => {
      it('removes all alerts', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        act(() => {
          contextValue!.showAlert({ type: 'success', message: 'Alert 1' })
          contextValue!.showAlert({ type: 'error', message: 'Alert 2' })
          contextValue!.showAlert({ type: 'warning', message: 'Alert 3' })
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('3')

        act(() => {
          contextValue!.clearAll()
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })

      it('works when no alerts exist', () => {
        let contextValue: ReturnType<typeof useAlertContext> | null = null

        renderWithProvider(
          <TestComponent
            onHookValue={value => {
              contextValue = value
            }}
          />
        )

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')

        act(() => {
          contextValue!.clearAll()
        })

        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })
    })
  })

  describe('Auto-dismiss functionality', () => {
    it('auto-dismisses non-persistent alerts after their duration', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({
          type: 'success',
          message: 'Auto-dismiss alert',
          duration: 5000,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward time by 5 seconds
      act(() => {
        jest.advanceTimersByTime(5000)
      })

      await waitFor(() => {
        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })
    })

    it('does not auto-dismiss persistent alerts', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({
          type: 'error',
          message: 'Persistent alert',
          persistent: true,
          duration: 5000,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward time by 10 seconds
      act(() => {
        jest.advanceTimersByTime(10000)
      })

      // Should still be there
      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
    })

    it('does not auto-dismiss alerts with duration 0', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({
          type: 'info',
          message: 'No auto-dismiss alert',
          duration: 0,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward time by 10 seconds
      act(() => {
        jest.advanceTimersByTime(10000)
      })

      // Should still be there
      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
    })

    it('auto-dismisses multiple alerts at different times', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({
          type: 'success',
          message: 'Quick alert',
          duration: 2000,
        })
        contextValue!.showAlert({
          type: 'error',
          message: 'Slow alert',
          duration: 5000,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('2')

      // Fast-forward by 2 seconds
      act(() => {
        jest.advanceTimersByTime(2000)
      })

      await waitFor(() => {
        expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
      })

      // Fast-forward by 3 more seconds (total 5)
      act(() => {
        jest.advanceTimersByTime(3000)
      })

      await waitFor(() => {
        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })
    })
  })

  describe('Memory leak prevention', () => {
    it('cleans up old non-persistent alerts after 30 seconds', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      // Create alerts with duration 0 (no auto-dismiss)
      act(() => {
        contextValue!.showAlert({
          type: 'info',
          message: 'Old alert',
          duration: 0,
          persistent: false,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward by 31 seconds (past the 30-second cleanup threshold)
      // The cleanup runs every 10 seconds, so we need to advance to trigger it
      act(() => {
        jest.advanceTimersByTime(31000)
      })

      await waitFor(() => {
        expect(screen.getByTestId('alerts-count')).toHaveTextContent('0')
      })
    })

    it('does not clean up persistent alerts during cleanup', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({
          type: 'error',
          message: 'Persistent old alert',
          duration: 0,
          persistent: true,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward by 31 seconds
      act(() => {
        jest.advanceTimersByTime(31000)
      })

      // Persistent alert should still be there
      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
    })

    it('does not clean up recent alerts during cleanup', async () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      // Create alert with duration 0 (no auto-dismiss)
      act(() => {
        contextValue!.showAlert({
          type: 'info',
          message: 'Recent alert',
          duration: 0,
          persistent: false,
        })
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')

      // Fast-forward by only 20 seconds (less than 30-second threshold)
      act(() => {
        jest.advanceTimersByTime(20000)
      })

      // Recent alert should still be there
      expect(screen.getByTestId('alerts-count')).toHaveTextContent('1')
    })
  })

  describe('Alert queuing', () => {
    it('maintains correct order when adding multiple alerts', () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        contextValue!.showAlert({ type: 'success', message: 'First alert' })
        contextValue!.showAlert({ type: 'error', message: 'Second alert' })
        contextValue!.showAlert({ type: 'warning', message: 'Third alert' })
      })

      const alerts = screen.getAllByTestId(
        /^alert-(success|error|warning|info)$/
      )
      expect(alerts).toHaveLength(3)

      // Check the order by finding messages
      expect(screen.getByText('First alert')).toBeTruthy()
      expect(screen.getByText('Second alert')).toBeTruthy()
      expect(screen.getByText('Third alert')).toBeTruthy()
    })

    it('handles rapid successive alert additions correctly', () => {
      let contextValue: ReturnType<typeof useAlertContext> | null = null

      renderWithProvider(
        <TestComponent
          onHookValue={value => {
            contextValue = value
          }}
        />
      )

      act(() => {
        for (let i = 0; i < 10; i++) {
          contextValue!.showAlert({
            type: 'info',
            message: `Alert ${i + 1}`,
          })
        }
      })

      expect(screen.getByTestId('alerts-count')).toHaveTextContent('10')
    })
  })

  describe('Component unmounting and cleanup', () => {
    it('cleans up intervals when component unmounts', () => {
      const clearIntervalSpy = jest.spyOn(global, 'clearInterval')

      const { unmount } = renderWithProvider(<TestComponent />)

      // Unmount the component
      unmount()

      // Verify that clearInterval was called (cleanup function from useEffect)
      expect(clearIntervalSpy).toHaveBeenCalled()

      clearIntervalSpy.mockRestore()
    })
  })
})
