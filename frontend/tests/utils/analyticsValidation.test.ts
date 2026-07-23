import {
  validateUserProperties,
  validateBaseEvent,
  validateTrackEventParams,
  validateTrackPageParams,
  validateTrackAuthParams,
  validateGTMCompatibility,
  validateAnalyticsEvent,
  logValidationResults,
  type ValidationResult,
} from '../../src/utils/analyticsValidation'
import type { AnalyticsEvent, UserProperties } from '../../src/types/analytics'
import { AUTH_EVENT_TYPES } from '../../src/types/analytics'

describe('analyticsValidation', () => {
  describe('validateUserProperties', () => {
    it('returns valid for complete user properties', () => {
      const userProps: UserProperties = {
        user_id: 'user-123',
        email: 'test@example.com',
        name: 'Test User',
      }

      const result = validateUserProperties(userProps)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('returns error when user_id is missing', () => {
      const userProps = {
        email: 'test@example.com',
        name: 'Test User',
      } as UserProperties

      const result = validateUserProperties(userProps)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'user_id is required and must be a string'
      )
    })

    it('returns error when email is missing', () => {
      const userProps = {
        user_id: 'user-123',
        name: 'Test User',
      } as UserProperties

      const result = validateUserProperties(userProps)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain('email is required and must be a string')
    })

    it('returns error when name is missing', () => {
      const userProps = {
        user_id: 'user-123',
        email: 'test@example.com',
      } as UserProperties

      const result = validateUserProperties(userProps)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain('name is required and must be a string')
    })

    it('returns warning for invalid email format', () => {
      const userProps: UserProperties = {
        user_id: 'user-123',
        email: 'invalid-email',
        name: 'Test User',
      }

      const result = validateUserProperties(userProps)

      expect(result.isValid).toBe(true) // Email format is a warning, not error
      expect(result.warnings).toContain('email format appears invalid')
    })

    it('returns warning for non-string signup_date', () => {
      const userProps = {
        user_id: 'user-123',
        email: 'test@example.com',
        name: 'Test User',
        signup_date: 12345 as unknown as string,
      }

      const result = validateUserProperties(userProps)

      expect(result.warnings).toContain(
        'signup_date should be a string (ISO date)'
      )
    })

    it('returns warning for non-string avatar_url', () => {
      const userProps = {
        user_id: 'user-123',
        email: 'test@example.com',
        name: 'Test User',
        avatar_url: 123 as unknown as string,
      }

      const result = validateUserProperties(userProps)

      expect(result.warnings).toContain('avatar_url should be a string')
    })
  })

  describe('validateBaseEvent', () => {
    const validEvent = {
      event: 'page_view',
      timestamp: Date.now(),
      page_path: '/test',
      page_title: 'Test Page',
      environment: 'production',
    } as AnalyticsEvent

    it('returns valid for complete base event', () => {
      const result = validateBaseEvent(validEvent)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('returns error when event name is missing', () => {
      const event = { ...validEvent, event: '' }

      const result = validateBaseEvent(event as AnalyticsEvent)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'event name is required and must be a string'
      )
    })

    it('returns error when timestamp is missing', () => {
      const event = { ...validEvent, timestamp: undefined as unknown as number }

      const result = validateBaseEvent(event)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'timestamp is required and must be a number'
      )
    })

    it('returns error when page_path is missing', () => {
      const event = { ...validEvent, page_path: '' }

      const result = validateBaseEvent(event)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'page_path is required and must be a string'
      )
    })

    it('returns warning for page_path not starting with /', () => {
      const event = { ...validEvent, page_path: 'test' }

      const result = validateBaseEvent(event)

      expect(result.warnings).toContain('page_path should start with "/"')
    })

    it('returns warning for unknown event name', () => {
      const event = { ...validEvent, event: 'unknown_custom_event' }

      const result = validateBaseEvent(event as AnalyticsEvent)

      expect(result.warnings).toContain(
        'Unknown event name: unknown_custom_event. Consider adding it to ANALYTICS_EVENTS'
      )
    })

    it('returns warning for timestamp too far in past', () => {
      const event = {
        ...validEvent,
        timestamp: Date.now() - 2 * 60 * 60 * 1000,
      } // 2 hours ago

      const result = validateBaseEvent(event)

      expect(result.warnings).toContain(
        'timestamp is more than 1 hour different from current time'
      )
    })

    it('validates user_properties if present', () => {
      const event = {
        ...validEvent,
        user_properties: {
          user_id: 'user-123',
          email: 'invalid',
          name: 'Test',
        },
      }

      const result = validateBaseEvent(event)

      expect(result.warnings).toContain('email format appears invalid')
    })
  })

  describe('validateTrackEventParams', () => {
    it('returns valid for complete event params', () => {
      const params = {
        event: 'button_click',
        properties: { button_id: 'submit' },
      }

      const result = validateTrackEventParams(params)

      expect(result.isValid).toBe(true)
    })

    it('returns error when event is missing', () => {
      const params = {
        event: '',
        properties: {},
      }

      const result = validateTrackEventParams(params)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'event name is required and must be a string'
      )
    })

    it('returns error when properties is not an object', () => {
      const params = {
        event: 'test_event',
        properties: 'not an object' as unknown as Record<string, unknown>,
      }

      const result = validateTrackEventParams(params)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain('properties must be an object')
    })
  })

  describe('validateTrackPageParams', () => {
    it('returns valid for complete page params', () => {
      const params = {
        path: '/dashboard',
        title: 'Dashboard',
      }

      const result = validateTrackPageParams(params)

      expect(result.isValid).toBe(true)
    })

    it('returns error when path is missing', () => {
      const params = {
        path: '',
        title: 'Dashboard',
      }

      const result = validateTrackPageParams(params)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain('path is required and must be a string')
    })

    it('returns error when title is missing', () => {
      const params = {
        path: '/dashboard',
        title: '',
      }

      const result = validateTrackPageParams(params)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain('title is required and must be a string')
    })

    it('returns warning for path not starting with /', () => {
      const params = {
        path: 'dashboard',
        title: 'Dashboard',
      }

      const result = validateTrackPageParams(params)

      expect(result.warnings).toContain('path should start with "/"')
    })

    it('returns warning for non-string referrer', () => {
      const params = {
        path: '/dashboard',
        title: 'Dashboard',
        referrer: 123 as unknown as string,
      }

      const result = validateTrackPageParams(params)

      expect(result.warnings).toContain('referrer should be a string')
    })
  })

  describe('validateTrackAuthParams', () => {
    it('returns valid for signin_page_view', () => {
      const params = { eventType: 'signin_page_view' as const }

      const result = validateTrackAuthParams(params)

      expect(result.isValid).toBe(true)
    })

    it('returns valid for signed_in', () => {
      const params = {
        eventType: 'signed_in' as const,
        userProperties: {
          user_id: 'user-123',
          email: 'test@example.com',
          name: 'Test',
        },
      }

      const result = validateTrackAuthParams(params)

      expect(result.isValid).toBe(true)
    })

    it('returns valid for logged_out', () => {
      const params = { eventType: 'logged_out' as const }

      const result = validateTrackAuthParams(params)

      expect(result.isValid).toBe(true)
    })

    // Exhaustiveness guard: every member of the TrackAuthParams union must
    // pass the validator, so the allowlist can never drift from the type
    // again (#388 — signed_in_first_time was silently dropped in dev).
    it.each(AUTH_EVENT_TYPES)('returns valid for %s', eventType => {
      const result = validateTrackAuthParams({ eventType })

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('returns error for invalid event type', () => {
      const params = { eventType: 'invalid_type' as 'signin_page_view' }

      const result = validateTrackAuthParams(params)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContain(
        'eventType must be one of: signin_page_view, signed_in, signed_in_first_time, logged_out'
      )
    })

    it('returns warning when signed_in has no user properties', () => {
      const params = { eventType: 'signed_in' as const }

      const result = validateTrackAuthParams(params)

      expect(result.warnings).toContain(
        'signed_in events should include user properties'
      )
    })
  })

  describe('validateGTMCompatibility', () => {
    it('returns valid for simple event data', () => {
      const eventData = {
        event: 'test_event',
        property: 'value',
      }

      const result = validateGTMCompatibility(eventData)

      expect(result.isValid).toBe(true)
    })

    it('returns warning for reserved GTM properties', () => {
      const eventData = {
        event: 'test_event',
        'gtm.start': 12345,
      }

      const result = validateGTMCompatibility(eventData)

      expect(result.warnings).toContain(
        'Property "gtm.start" is reserved by GTM and may be overwritten'
      )
    })

    it('returns warning for very long property names', () => {
      const longPropertyName = 'a'.repeat(101)
      const eventData = {
        event: 'test_event',
        [longPropertyName]: 'value',
      }

      const result = validateGTMCompatibility(eventData)

      expect(result.warnings).toContain(
        `Property name "${longPropertyName}" is very long and may be truncated by GTM`
      )
    })

    it('returns warning for deeply nested data', () => {
      const eventData = {
        event: 'test_event',
        level1: {
          level2: {
            level3: {
              level4: {
                level5: {
                  level6: 'deep value',
                },
              },
            },
          },
        },
      }

      const result = validateGTMCompatibility(eventData)

      expect(result.warnings).toContain(
        'Event data is deeply nested and may not be fully processed by GTM'
      )
    })
  })

  describe('validateAnalyticsEvent', () => {
    it('combines base event and GTM validation', () => {
      const event = {
        event: 'page_view',
        timestamp: Date.now(),
        page_path: '/test',
        page_title: 'Test Page',
        environment: 'production',
      } as AnalyticsEvent

      const result = validateAnalyticsEvent(event)

      expect(result.isValid).toBe(true)
    })

    it('combines errors from both validations', () => {
      const event = {
        event: '',
        timestamp: undefined as unknown as number,
        page_path: '',
        environment: '',
      } as unknown as AnalyticsEvent

      const result = validateAnalyticsEvent(event)

      expect(result.isValid).toBe(false)
      expect(result.errors.length).toBeGreaterThan(0)
    })
  })

  describe('logValidationResults', () => {
    let consoleLogSpy: jest.SpyInstance
    let consoleGroupSpy: jest.SpyInstance
    let consoleErrorSpy: jest.SpyInstance
    let consoleWarnSpy: jest.SpyInstance

    beforeEach(() => {
      consoleLogSpy = jest.spyOn(console, 'log').mockImplementation()
      consoleGroupSpy = jest.spyOn(console, 'group').mockImplementation()
      jest.spyOn(console, 'groupEnd').mockImplementation()
      consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation()
      consoleWarnSpy = jest.spyOn(console, 'warn').mockImplementation()
    })

    afterEach(() => {
      jest.restoreAllMocks()
    })

    it('logs valid event without warnings', () => {
      const result: ValidationResult = {
        isValid: true,
        errors: [],
        warnings: [],
      }

      logValidationResults('test_event', result, true)

      expect(consoleLogSpy).toHaveBeenCalledWith(
        expect.stringContaining('test_event - Valid')
      )
    })

    it('logs valid event with warnings', () => {
      const result: ValidationResult = {
        isValid: true,
        errors: [],
        warnings: ['A warning'],
      }

      logValidationResults('test_event', result, true)

      expect(consoleGroupSpy).toHaveBeenCalled()
      expect(consoleWarnSpy).toHaveBeenCalledWith('Warning:', 'A warning')
    })

    it('logs invalid event with errors', () => {
      const result: ValidationResult = {
        isValid: false,
        errors: ['An error'],
        warnings: [],
      }

      logValidationResults('test_event', result, true)

      expect(consoleGroupSpy).toHaveBeenCalled()
      expect(consoleErrorSpy).toHaveBeenCalledWith('Error:', 'An error')
    })

    it('does not log when logging is disabled', () => {
      const result: ValidationResult = {
        isValid: false,
        errors: ['An error'],
        warnings: [],
      }

      logValidationResults('test_event', result, false)

      expect(consoleLogSpy).not.toHaveBeenCalled()
      expect(consoleGroupSpy).not.toHaveBeenCalled()
    })
  })
})
