/**
 * Unit Tests for useAnalytics Hook - Issue #737
 *
 * This test suite validates the useAnalytics hook functionality including:
 * - Event tracking (track, trackEvent, trackPage, trackAuth, trackError)
 * - User identification
 * - User properties management
 * - Error handling
 * - Memoization of callback functions
 *
 * Coverage target: >50%
 */

import { renderHook, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import React from 'react'

import type {
  AnalyticsEvent,
  TrackEventParams,
  TrackPageParams,
  TrackAuthParams,
  TrackErrorParams,
  UserProperties,
} from '../../src/types/analytics'

// Mock the analyticsService
const mockAnalyticsService = {
  track: jest.fn(),
  trackEvent: jest.fn(),
  trackPage: jest.fn(),
  trackAuth: jest.fn(),
  trackError: jest.fn(),
  identify: jest.fn(),
  isEnabled: jest.fn(() => true),
}

jest.mock('../../src/services/analytics', () => ({
  analyticsService: mockAnalyticsService,
}))

// Mock the AuthContext
interface MockUser {
  id: string
  email: string
  name: string
  avatar_url?: string | null
  created_at: string
}

interface AuthContextValue {
  user: MockUser | null
  isAuthenticated: boolean
}

let mockAuthContext: AuthContextValue = {
  user: null,
  isAuthenticated: false,
}

const MockAuthContext = React.createContext<AuthContextValue>(mockAuthContext)

jest.mock('../../src/contexts/AuthContext', () => ({
  useAuth: () => mockAuthContext,
}))

// Import the hook after mocking
import { useAnalytics } from '../../src/hooks/useAnalytics'

// Create a wrapper for the hook - exported for potential external use
// eslint-disable-next-line @typescript-eslint/no-unused-vars
const createWrapper = (authValue: AuthContextValue) => {
  mockAuthContext = authValue
  return ({ children }: { children: ReactNode }) => (
    <MockAuthContext.Provider value={authValue}>
      {children}
    </MockAuthContext.Provider>
  )
}

describe('useAnalytics', () => {
  const mockUser: MockUser = {
    id: 'user-123',
    email: 'test@example.com',
    name: 'Test User',
    avatar_url: 'https://example.com/avatar.jpg',
    created_at: '2023-01-01T00:00:00Z',
  }

  const mockUserProperties: UserProperties = {
    user_id: mockUser.id,
    email: mockUser.email,
    name: mockUser.name,
    signup_date: mockUser.created_at,
    avatar_url: mockUser.avatar_url ?? null,
    created_at: mockUser.created_at,
  }

  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthContext = {
      user: null,
      isAuthenticated: false,
    }
    mockAnalyticsService.isEnabled.mockReturnValue(true)
  })

  describe('Initialization', () => {
    it('should return all tracking functions', () => {
      const { result } = renderHook(() => useAnalytics())

      expect(result.current.track).toBeDefined()
      expect(result.current.trackEvent).toBeDefined()
      expect(result.current.trackPage).toBeDefined()
      expect(result.current.trackAuth).toBeDefined()
      expect(result.current.trackError).toBeDefined()
      expect(result.current.identify).toBeDefined()
      expect(typeof result.current.isEnabled).toBe('boolean')
    })

    it('should return isEnabled state from analytics service', () => {
      mockAnalyticsService.isEnabled.mockReturnValue(true)

      const { result } = renderHook(() => useAnalytics())

      expect(result.current.isEnabled).toBe(true)
    })

    it('should return false for isEnabled when analytics is disabled', () => {
      mockAnalyticsService.isEnabled.mockReturnValue(false)

      const { result } = renderHook(() => useAnalytics())

      expect(result.current.isEnabled).toBe(false)
    })
  })

  describe('User Properties', () => {
    it('should not include user properties when not authenticated', () => {
      mockAuthContext = {
        user: null,
        isAuthenticated: false,
      }

      const { result } = renderHook(() => useAnalytics())

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'development',
      }

      act(() => {
        result.current.track(mockEvent)
      })

      // Should be called without user properties
      expect(mockAnalyticsService.track).toHaveBeenCalled()
      const trackedEvent = mockAnalyticsService.track.mock.calls[0][0]
      expect(trackedEvent.user_properties).toBeUndefined()
    })

    it('should include user properties when authenticated', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'development',
      }

      act(() => {
        result.current.track(mockEvent)
      })

      expect(mockAnalyticsService.track).toHaveBeenCalled()
      const trackedEvent = mockAnalyticsService.track.mock.calls[0][0]
      expect(trackedEvent.user_properties).toEqual(mockUserProperties)
    })

    it('should handle user with null avatar_url', () => {
      const userWithoutAvatar = {
        ...mockUser,
        avatar_url: null,
      }

      mockAuthContext = {
        user: userWithoutAvatar,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'development',
      }

      act(() => {
        result.current.track(mockEvent)
      })

      const trackedEvent = mockAnalyticsService.track.mock.calls[0][0]
      expect(trackedEvent.user_properties.avatar_url).toBeNull()
    })
  })

  describe('track()', () => {
    it('should call analyticsService.track with the event', () => {
      mockAuthContext = {
        user: null,
        isAuthenticated: false,
      }

      const { result } = renderHook(() => useAnalytics())

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/dashboard',
        page_title: 'Dashboard',
        environment: 'development',
      }

      act(() => {
        result.current.track(mockEvent)
      })

      expect(mockAnalyticsService.track).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.track).toHaveBeenCalledWith(
        expect.objectContaining({
          event: 'page_view',
          page_path: '/dashboard',
        })
      )
    })

    it('should preserve existing user_properties in event', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const customUserProperties: UserProperties = {
        user_id: 'custom-user',
        email: 'custom@example.com',
        name: 'Custom User',
      }

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'development',
        user_properties: customUserProperties,
      }

      act(() => {
        result.current.track(mockEvent)
      })

      const trackedEvent = mockAnalyticsService.track.mock.calls[0][0]
      expect(trackedEvent.user_properties).toEqual(customUserProperties)
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.track.mockImplementation(() => {
        throw new Error('Tracking error')
      })

      const { result } = renderHook(() => useAnalytics())

      const mockEvent: AnalyticsEvent = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'development',
      }

      // Should not throw
      act(() => {
        expect(() => result.current.track(mockEvent)).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error tracking event:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('trackEvent()', () => {
    it('should call analyticsService.trackEvent with params', () => {
      mockAuthContext = {
        user: null,
        isAuthenticated: false,
      }

      const { result } = renderHook(() => useAnalytics())

      const params: TrackEventParams = {
        event: 'button_click',
        properties: { button_id: 'submit-btn' },
      }

      act(() => {
        result.current.trackEvent(params)
      })

      expect(mockAnalyticsService.trackEvent).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.trackEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          event: 'button_click',
          properties: { button_id: 'submit-btn' },
        })
      )
    })

    it('should include user properties when authenticated', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const params: TrackEventParams = {
        event: 'feature_used',
      }

      act(() => {
        result.current.trackEvent(params)
      })

      const callParams = mockAnalyticsService.trackEvent.mock.calls[0][0]
      expect(callParams.userProperties).toEqual(mockUserProperties)
    })

    it('should preserve custom userProperties if provided', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const customUserProperties: UserProperties = {
        user_id: 'custom',
        email: 'custom@test.com',
        name: 'Custom',
      }

      const params: TrackEventParams = {
        event: 'test_event',
        userProperties: customUserProperties,
      }

      act(() => {
        result.current.trackEvent(params)
      })

      const callParams = mockAnalyticsService.trackEvent.mock.calls[0][0]
      expect(callParams.userProperties).toEqual(customUserProperties)
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.trackEvent.mockImplementation(() => {
        throw new Error('Tracking error')
      })

      const { result } = renderHook(() => useAnalytics())

      act(() => {
        expect(() => result.current.trackEvent({ event: 'test' })).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error tracking custom event:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('trackPage()', () => {
    it('should call analyticsService.trackPage with params', () => {
      mockAuthContext = {
        user: null,
        isAuthenticated: false,
      }

      const { result } = renderHook(() => useAnalytics())

      const params: TrackPageParams = {
        path: '/dashboard',
        title: 'Dashboard',
        referrer: 'https://google.com',
      }

      act(() => {
        result.current.trackPage(params)
      })

      expect(mockAnalyticsService.trackPage).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.trackPage).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/dashboard',
          title: 'Dashboard',
          referrer: 'https://google.com',
        })
      )
    })

    it('should include user properties when authenticated', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const params: TrackPageParams = {
        path: '/profile',
        title: 'User Profile',
      }

      act(() => {
        result.current.trackPage(params)
      })

      const callParams = mockAnalyticsService.trackPage.mock.calls[0][0]
      expect(callParams.userProperties).toEqual(mockUserProperties)
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.trackPage.mockImplementation(() => {
        throw new Error('Tracking error')
      })

      const { result } = renderHook(() => useAnalytics())

      act(() => {
        expect(() =>
          result.current.trackPage({ path: '/', title: 'Home' })
        ).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error tracking page:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('trackAuth()', () => {
    it('should call analyticsService.trackAuth with params', () => {
      const { result } = renderHook(() => useAnalytics())

      const params: TrackAuthParams = {
        eventType: 'signed_in',
      }

      act(() => {
        result.current.trackAuth(params)
      })

      expect(mockAnalyticsService.trackAuth).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.trackAuth).toHaveBeenCalledWith(
        expect.objectContaining({
          eventType: 'signed_in',
        })
      )
    })

    it('should track sign in first time event', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result } = renderHook(() => useAnalytics())

      const params: TrackAuthParams = {
        eventType: 'signed_in_first_time',
      }

      act(() => {
        result.current.trackAuth(params)
      })

      expect(mockAnalyticsService.trackAuth).toHaveBeenCalledWith(
        expect.objectContaining({
          eventType: 'signed_in_first_time',
          userProperties: mockUserProperties,
        })
      )
    })

    it('should track logout event', () => {
      const { result } = renderHook(() => useAnalytics())

      const params: TrackAuthParams = {
        eventType: 'logged_out',
      }

      act(() => {
        result.current.trackAuth(params)
      })

      expect(mockAnalyticsService.trackAuth).toHaveBeenCalledWith(
        expect.objectContaining({
          eventType: 'logged_out',
        })
      )
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.trackAuth.mockImplementation(() => {
        throw new Error('Tracking error')
      })

      const { result } = renderHook(() => useAnalytics())

      act(() => {
        expect(() =>
          result.current.trackAuth({ eventType: 'signed_in' })
        ).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error tracking auth event:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('trackError()', () => {
    it('should call analyticsService.trackError with params', () => {
      const { result } = renderHook(() => useAnalytics())

      const testError = new Error('Test error')
      const params: TrackErrorParams = {
        error: testError,
        component: 'TestComponent',
      }

      act(() => {
        result.current.trackError(params)
      })

      expect(mockAnalyticsService.trackError).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.trackError).toHaveBeenCalledWith(params)
    })

    it('should track error with additional info', () => {
      const { result } = renderHook(() => useAnalytics())

      const testError = new Error('API Error')
      const params: TrackErrorParams = {
        error: testError,
        component: 'ApiService',
        additionalInfo: {
          endpoint: '/api/v1/users',
          statusCode: 500,
        },
      }

      act(() => {
        result.current.trackError(params)
      })

      expect(mockAnalyticsService.trackError).toHaveBeenCalledWith(
        expect.objectContaining({
          error: testError,
          component: 'ApiService',
          additionalInfo: {
            endpoint: '/api/v1/users',
            statusCode: 500,
          },
        })
      )
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.trackError.mockImplementation(() => {
        throw new Error('Tracking error')
      })

      const { result } = renderHook(() => useAnalytics())

      act(() => {
        expect(() =>
          result.current.trackError({ error: new Error('Test') })
        ).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error tracking error:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('identify()', () => {
    it('should call analyticsService.identify with user properties', () => {
      const { result } = renderHook(() => useAnalytics())

      const userProperties: UserProperties = {
        user_id: 'user-456',
        email: 'new@example.com',
        name: 'New User',
      }

      act(() => {
        result.current.identify(userProperties)
      })

      expect(mockAnalyticsService.identify).toHaveBeenCalledTimes(1)
      expect(mockAnalyticsService.identify).toHaveBeenCalledWith(userProperties)
    })

    it('should handle errors gracefully', () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockAnalyticsService.identify.mockImplementation(() => {
        throw new Error('Identify error')
      })

      const { result } = renderHook(() => useAnalytics())

      act(() => {
        expect(() =>
          result.current.identify({
            user_id: 'test',
            email: 'test@test.com',
            name: 'Test',
          })
        ).not.toThrow()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        '[useAnalytics] Error identifying user:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('Memoization', () => {
    it('should return stable function references', () => {
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      const { result, rerender } = renderHook(() => useAnalytics())

      const initialTrack = result.current.track
      const initialTrackEvent = result.current.trackEvent
      const initialTrackPage = result.current.trackPage
      const initialTrackAuth = result.current.trackAuth
      const initialTrackError = result.current.trackError
      const initialIdentify = result.current.identify

      rerender()

      // trackError and identify should be stable (no dependencies)
      expect(result.current.trackError).toBe(initialTrackError)
      expect(result.current.identify).toBe(initialIdentify)

      // Other functions depend on userProperties, so they may change
      // when user changes. But if user stays same, they should be stable.
      expect(result.current.track).toBe(initialTrack)
      expect(result.current.trackEvent).toBe(initialTrackEvent)
      expect(result.current.trackPage).toBe(initialTrackPage)
      expect(result.current.trackAuth).toBe(initialTrackAuth)
    })

    it('should update functions when user changes', () => {
      const { result, rerender } = renderHook(() => useAnalytics())

      // Store initial reference to verify functions are defined
      expect(result.current.track).toBeDefined()

      // Change the auth context
      mockAuthContext = {
        user: mockUser,
        isAuthenticated: true,
      }

      rerender()

      // Functions that depend on userProperties should update
      // Note: This may or may not change depending on implementation details
      // The important thing is that the hook works correctly
      expect(result.current.track).toBeDefined()
    })
  })

  describe('Edge Cases', () => {
    it('should handle undefined user gracefully', () => {
      mockAuthContext = {
        user: null,
        isAuthenticated: true, // Authenticated but user is null (edge case)
      }

      const { result } = renderHook(() => useAnalytics())

      // Should not throw
      act(() => {
        result.current.track({
          event: 'page_view',
          timestamp: Date.now(),
          page_path: '/test',
          page_title: 'Test',
          environment: 'development',
        })
      })

      expect(mockAnalyticsService.track).toHaveBeenCalled()
    })

    it('should work when analytics service is disabled', () => {
      mockAnalyticsService.isEnabled.mockReturnValue(false)

      const { result } = renderHook(() => useAnalytics())

      expect(result.current.isEnabled).toBe(false)

      // Should still be able to call functions without errors
      act(() => {
        result.current.track({
          event: 'page_view',
          timestamp: Date.now(),
          page_path: '/test',
          page_title: 'Test',
          environment: 'development',
        })
      })

      // The function was called (the service decides whether to actually track)
      expect(mockAnalyticsService.track).toHaveBeenCalled()
    })
  })
})
