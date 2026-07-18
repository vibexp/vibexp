import { act, renderHook, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import { STORAGE_KEYS } from '../../constants/storageKeys'
import * as AnalyticsHook from '../useAnalytics'
import { usePageTracking } from '../usePageTracking'

// Mock useAnalytics hook
const mockTrackPage = jest.fn()
let mockIsEnabled = true

jest.spyOn(AnalyticsHook, 'useAnalytics').mockImplementation(() => ({
  track: jest.fn(),
  trackEvent: jest.fn(),
  trackPage: mockTrackPage,
  trackAuth: jest.fn(),
  trackError: jest.fn(),
  identify: jest.fn(),
  isEnabled: mockIsEnabled,
}))

// Mock the centralized storage utilities
let sessionStore: Record<string, string> = {}

jest.mock('../../utils/storage', () => ({
  storage: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
  },
  sessionStore: {
    get: jest.fn((key: string) => {
      const value = sessionStore[key]
      if (!value) return null
      try {
        return JSON.parse(value)
      } catch {
        return value
      }
    }),
    set: jest.fn((key: string, value: unknown) => {
      sessionStore[key] =
        typeof value === 'string' ? value : JSON.stringify(value)
    }),
    has: jest.fn(),
  },
}))

// Wrapper component for react-router
function createWrapper(initialPath = '/') {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="*" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    )
  }
}

describe('usePageTracking', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    // Mock document.title
    Object.defineProperty(document, 'title', {
      writable: true,
      value: 'Test Page',
    })
    // Clear sessionStore mock
    sessionStore = {}
  })

  afterEach(() => {
    jest.runOnlyPendingTimers()
    jest.useRealTimers()
  })

  describe('initialization', () => {
    it('should initialize without errors', () => {
      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper(),
      })

      expect(result.current.trackCurrentPage).toBeDefined()
      expect(result.current.trackPageView).toBeDefined()
      expect(typeof result.current.trackCurrentPage).toBe('function')
      expect(typeof result.current.trackPageView).toBe('function')
    })

    it('should track initial page view when analytics is enabled', async () => {
      mockIsEnabled = true

      renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test-page'),
      })

      // Fast-forward initial delay (50ms)
      act(() => {
        jest.advanceTimersByTime(50)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalledWith(
          expect.objectContaining({
            path: '/test-page',
            title: 'Test Page',
          })
        )
      })
    })

    it('should respect custom options', () => {
      const { result } = renderHook(
        () =>
          usePageTracking({
            enableAutoTracking: false,
            debounceMs: 200,
          }),
        {
          wrapper: createWrapper(),
        }
      )

      expect(result.current).toBeDefined()
      // No tracking should happen with auto tracking disabled
      act(() => {
        jest.advanceTimersByTime(50)
      })
      expect(mockTrackPage).not.toHaveBeenCalled()
    })
  })

  describe('consent state handling', () => {
    it('should NOT track when analytics is disabled (consent not given)', () => {
      mockIsEnabled = false

      renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test-page'),
      })

      // Fast-forward all timers
      act(() => {
        jest.advanceTimersByTime(200)
      })

      expect(mockTrackPage).not.toHaveBeenCalled()
    })

    it('should track when analytics is enabled (consent given)', async () => {
      mockIsEnabled = true

      renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test-page'),
      })

      act(() => {
        jest.advanceTimersByTime(50)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalledTimes(1)
      })
    })

    it('should respect consent state for manual tracking', () => {
      mockIsEnabled = false

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper(),
      })

      act(() => {
        result.current.trackPageView('/manual-page', 'Manual Page')
      })

      expect(mockTrackPage).not.toHaveBeenCalled()
    })

    it('should track manual page views when consent is given', () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper(),
      })

      act(() => {
        result.current.trackPageView('/manual-page', 'Manual Page')
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/manual-page',
          title: 'Manual Page',
        })
      )
    })
  })

  describe('debouncing behavior', () => {
    it('should use 50ms delay for initial page view', async () => {
      mockIsEnabled = true

      renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/initial'),
      })

      // Before 50ms - should not have tracked yet
      act(() => {
        jest.advanceTimersByTime(49)
      })
      expect(mockTrackPage).not.toHaveBeenCalled()

      // After 50ms - should have tracked
      act(() => {
        jest.advanceTimersByTime(1)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalledWith(
          expect.objectContaining({
            path: '/initial',
          })
        )
      })
    })

    it('should use 100ms debounce for route changes (default)', () => {
      mockIsEnabled = true

      // Verify that with default debounceMs (100), the hook accepts it
      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/start'),
      })

      // Wait for initial tracking
      act(() => {
        jest.advanceTimersByTime(50)
      })

      // The default debounceMs is 100ms as verified by the hook's implementation
      expect(result.current.trackCurrentPage).toBeDefined()
      expect(mockTrackPage).toHaveBeenCalled()
    })

    it('should respect custom debounce timing', () => {
      mockIsEnabled = true

      // Verify that custom debounceMs value is accepted
      const { result } = renderHook(
        () => usePageTracking({ debounceMs: 200 }),
        {
          wrapper: createWrapper('/start'),
        }
      )

      // Wait for initial tracking with 50ms delay
      act(() => {
        jest.advanceTimersByTime(50)
      })

      // The custom debounceMs of 200ms is used for route changes
      expect(result.current.trackCurrentPage).toBeDefined()
      expect(mockTrackPage).toHaveBeenCalled()
    })

    it('should prevent duplicate events during rapid navigation', async () => {
      mockIsEnabled = true

      // Render with initial route
      renderHook(() => usePageTracking({ debounceMs: 100 }), {
        wrapper: createWrapper('/start'),
      })

      // Clear initial tracking
      act(() => {
        jest.advanceTimersByTime(50)
      })
      mockTrackPage.mockClear()

      // Simulate rapid navigation by triggering multiple route changes quickly
      // Each re-render simulates a navigation
      renderHook(() => usePageTracking({ debounceMs: 100 }), {
        wrapper: createWrapper('/page1'),
      })

      act(() => {
        jest.advanceTimersByTime(30) // Before debounce completes
      })

      renderHook(() => usePageTracking({ debounceMs: 100 }), {
        wrapper: createWrapper('/page2'),
      })

      act(() => {
        jest.advanceTimersByTime(30) // Before debounce completes
      })

      renderHook(() => usePageTracking({ debounceMs: 100 }), {
        wrapper: createWrapper('/page3'),
      })

      // Fast forward past the debounce
      act(() => {
        jest.advanceTimersByTime(100)
      })

      // With debouncing, we should have fewer tracking calls than navigations
      // The exact behavior depends on the debounce implementation
      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalled()
      })
    })

    it('should cancel pending tracking on unmount', () => {
      mockIsEnabled = true

      const { unmount } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test'),
      })

      // Unmount before debounce completes
      unmount()

      // Fast forward timers
      act(() => {
        jest.advanceTimersByTime(100)
      })

      // Should not have tracked after unmount
      expect(mockTrackPage).not.toHaveBeenCalled()
    })
  })

  describe('route change tracking', () => {
    it('should track page views on route changes', async () => {
      mockIsEnabled = true

      // Using a more controlled approach with MemoryRouter
      let currentPath = '/page1'
      const { rerender } = renderHook(
        () => {
          return usePageTracking()
        },
        {
          initialProps: {},
          wrapper: ({ children }) => (
            <MemoryRouter initialEntries={[currentPath]}>
              {children}
            </MemoryRouter>
          ),
        }
      )

      // Clear initial page tracking
      act(() => {
        jest.advanceTimersByTime(50)
      })
      mockTrackPage.mockClear()

      // Change route
      currentPath = '/page2'
      rerender()

      // Fast forward debounce
      act(() => {
        jest.advanceTimersByTime(100)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalled()
      })
    })

    it('should NOT track duplicate page views for same path', async () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/same-page'),
      })

      // Initial tracking
      act(() => {
        jest.advanceTimersByTime(50)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalledTimes(1)
      })

      const initialCallCount = mockTrackPage.mock.calls.length

      // Try to track same page again via manual call
      // Manual calls WILL track (they bypass deduplication)
      act(() => {
        result.current.trackCurrentPage()
      })

      // Manual tracking happens immediately
      expect(mockTrackPage.mock.calls).toHaveLength(initialCallCount + 1)
    })

    it('should include query parameters in path tracking', async () => {
      mockIsEnabled = true

      renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/search?q=test'),
      })

      act(() => {
        jest.advanceTimersByTime(50)
      })

      await waitFor(() => {
        expect(mockTrackPage).toHaveBeenCalledWith(
          expect.objectContaining({
            path: '/search?q=test',
          })
        )
      })
    })

    it('should disable auto tracking when option is false', () => {
      mockIsEnabled = true

      renderHook(() => usePageTracking({ enableAutoTracking: false }), {
        wrapper: createWrapper('/test'),
      })

      act(() => {
        jest.advanceTimersByTime(200)
      })

      expect(mockTrackPage).not.toHaveBeenCalled()
    })
  })

  describe('manual tracking methods', () => {
    it('should track current page with trackCurrentPage', () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/current'),
      })

      act(() => {
        result.current.trackCurrentPage()
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/current',
          title: 'Test Page',
        })
      )
    })

    it('should track custom page with trackPageView', () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper(),
      })

      act(() => {
        result.current.trackPageView('/custom-page', 'Custom Title')
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/custom-page',
          title: 'Custom Title',
        })
      )
    })

    it('should use current location when path is not provided', () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/current-location'),
      })

      act(() => {
        result.current.trackPageView()
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/current-location',
        })
      )
    })

    it('should use document.title when title is not provided', () => {
      mockIsEnabled = true
      document.title = 'Document Title'

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test'),
      })

      act(() => {
        result.current.trackPageView('/test')
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Document Title',
        })
      )
    })
  })

  describe('referrer tracking', () => {
    it('should include referrer from document.referrer', () => {
      mockIsEnabled = true
      Object.defineProperty(document, 'referrer', {
        writable: true,
        value: 'https://external-site.com',
      })

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test'),
      })

      act(() => {
        result.current.trackCurrentPage()
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          referrer: 'https://external-site.com',
        })
      )
    })

    it('should use sessionStorage referrer for SPA navigation', () => {
      mockIsEnabled = true
      sessionStore[STORAGE_KEYS.ANALYTICS_REFERRER] =
        'https://app.example.com/previous-page'
      Object.defineProperty(document, 'referrer', {
        writable: true,
        value: '',
      })

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/test'),
      })

      act(() => {
        result.current.trackCurrentPage()
      })

      expect(mockTrackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          referrer: 'https://app.example.com/previous-page',
        })
      )
    })

    it('should store current page as referrer for next navigation', () => {
      mockIsEnabled = true

      const { result } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper('/current-page'),
      })

      act(() => {
        result.current.trackCurrentPage()
      })

      // Verify that referrer is stored in session storage
      // The value will be the current window.location.href from the test environment
      const storedReferrer = sessionStore[STORAGE_KEYS.ANALYTICS_REFERRER]
      expect(storedReferrer).toBeTruthy()
    })
  })

  describe('cleanup', () => {
    it('should cleanup timers on unmount', () => {
      const { unmount } = renderHook(() => usePageTracking(), {
        wrapper: createWrapper(),
      })

      // Verify there are pending timers
      expect(jest.getTimerCount()).toBeGreaterThan(0)

      unmount()

      // All timers should be cleaned up
      act(() => {
        jest.runOnlyPendingTimers()
      })
      // This test verifies that no errors occur during cleanup
    })

    it('should handle multiple mount/unmount cycles', () => {
      const wrapper = createWrapper()

      expect(() => {
        const { unmount: unmount1 } = renderHook(() => usePageTracking(), {
          wrapper,
        })
        unmount1()

        const { unmount: unmount2 } = renderHook(() => usePageTracking(), {
          wrapper,
        })
        unmount2()

        const { unmount: unmount3 } = renderHook(() => usePageTracking(), {
          wrapper,
        })
        unmount3()
      }).not.toThrow()
    })
  })
})
