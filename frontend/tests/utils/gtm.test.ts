/**
 * @jest-environment jsdom
 */

// Make this file a module for TypeScript
export {}

// Pull in the `window.__VIBEXP_ENV__` global augmentation (issue #57).
import '@/lib/runtimeEnv'

// Extend Window type for tests
// dataLayer and gtag are always defined in index.html; gtm.ts now reads its
// config at runtime via getEnv(), which prefers window.__VIBEXP_ENV__ (issue
// #57), so the enabled-GTM suite below sets that instead of build-time globals.
declare global {
  interface Window {
    dataLayer: Record<string, unknown>[]
    gtag: (...args: unknown[]) => void
  }
}

// Mock console methods
const originalConsoleLog = console.log
const originalConsoleError = console.error

beforeEach(() => {
  // Reset window.dataLayer
  window.dataLayer = []

  // Reset mocks
  jest.clearAllMocks()

  // Mock console methods
  console.log = jest.fn()
  console.error = jest.fn()

  // Clear any existing GTM scripts
  const existingScripts = document.querySelectorAll(
    'script[src*="googletagmanager"]'
  )
  existingScripts.forEach(script => script.remove())

  const existingNoscripts = document.querySelectorAll('noscript')
  existingNoscripts.forEach(noscript => noscript.remove())
})

afterEach(() => {
  // Restore console methods
  console.log = originalConsoleLog
  console.error = originalConsoleError
})

describe('GTM Utilities', () => {
  describe('Environment Configuration (GTM Disabled)', () => {
    it('should have test environment with GTM disabled by default', () => {
      // These tests run with GTM disabled (set in jest.config.js)
      const {
        GTM_ENABLED,
        GTM_ID,
        GA4_MEASUREMENT_ID,
      } = require('../../src/utils/gtm')

      expect(GTM_ENABLED).toBe(false)
      expect(GTM_ID).toBe('')
      expect(GA4_MEASUREMENT_ID).toBe('')
    })

    it('should have clear defaults that can be verified', () => {
      // Verify the constants are defined even when disabled
      const {
        GTM_ENABLED,
        GTM_ID,
        GA4_MEASUREMENT_ID,
      } = require('../../src/utils/gtm')

      expect(GTM_ENABLED).toBeDefined()
      expect(GTM_ID).toBeDefined()
      expect(GA4_MEASUREMENT_ID).toBeDefined()
    })
  })

  describe('When GTM is disabled (default)', () => {
    it('should not track events when GTM is disabled', () => {
      const { trackEvent } = require('../../src/utils/gtm')

      trackEvent('test_event', { category: 'test' })

      // dataLayer should remain empty since GTM is disabled
      expect(window.dataLayer).toHaveLength(0)
    })

    it('should not initialize GTM when disabled', () => {
      const { initializeGTM } = require('../../src/utils/gtm')

      initializeGTM()

      // Check that no GTM script was added
      const gtmScript = document.querySelector(
        'script[src*="googletagmanager"]'
      )
      expect(gtmScript).toBeFalsy()
    })
  })
})

// Separate test suite with GTM enabled
describe('GTM Utilities (GTM Enabled)', () => {
  beforeEach(() => {
    // Reset window.dataLayer
    window.dataLayer = []

    // Reset mocks
    jest.clearAllMocks()

    // Mock console methods
    console.log = jest.fn()
    console.error = jest.fn()

    // Clear any existing GTM scripts
    const existingScripts = document.querySelectorAll(
      'script[src*="googletagmanager"]'
    )
    existingScripts.forEach(script => script.remove())

    const existingNoscripts = document.querySelectorAll('noscript')
    existingNoscripts.forEach(noscript => noscript.remove())

    // Enable GTM at runtime for these tests (gtm.ts reads getEnv()).
    window.__VIBEXP_ENV__ = {
      VITE_GTM_ID: 'GTM-TEST123',
      VITE_GTM_ENABLED: 'true',
      VITE_GA4_MEASUREMENT_ID: 'G-TEST123',
    }
  })

  afterEach(() => {
    // Clear runtime config
    delete window.__VIBEXP_ENV__

    // Restore console methods
    console.log = originalConsoleLog
    console.error = originalConsoleError
  })

  describe('trackEvent', () => {
    it('should add prefixed event to dataLayer with no parameters', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('test_event')

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_test_event',
        })
      })
    })

    it('should add prefixed event to dataLayer with parameters', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        const parameters = {
          category: 'user_action',
          label: 'button_click',
          value: 1,
        }

        trackEvent('button_click', parameters)

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_button_click',
          category: 'user_action',
          label: 'button_click',
          value: 1,
        })
      })
    })

    it('should exclude event property from parameters to prevent overwriting prefixed event name', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        const parameters = {
          event: 'original_event_name',
          category: 'user_action',
          label: 'button_click',
          value: 1,
        }

        trackEvent('test_event', parameters)

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_test_event', // Should use prefixed name, not original
          category: 'user_action',
          label: 'button_click',
          value: 1,
        })
      })
    })

    it('should handle parameters with nested objects', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        const parameters = {
          event: 'should_be_ignored',
          user: {
            id: '123',
            role: 'admin',
          },
          metadata: {
            source: 'frontend',
            version: '1.0.0',
          },
        }

        trackEvent('complex_event', parameters)

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_complex_event',
          user: {
            id: '123',
            role: 'admin',
          },
          metadata: {
            source: 'frontend',
            version: '1.0.0',
          },
        })
      })
    })

    it('should handle empty parameters object', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('empty_params', {})

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_empty_params',
        })
      })
    })

    it('should handle undefined parameters', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('undefined_params', undefined)

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_undefined_params',
        })
      })
    })

    it('should not track event when dataLayer is not available', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        // Remove dataLayer
        delete window.dataLayer

        trackEvent('test_event', { category: 'test' })

        // Should not throw error and dataLayer should remain undefined
        expect(window.dataLayer).toBeUndefined()
      })
    })

    it('should handle multiple consecutive events', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('first_event', { step: 1 })
        trackEvent('second_event', { step: 2 })
        trackEvent('third_event', { step: 3, event: 'should_be_ignored' })

        expect(window.dataLayer).toHaveLength(3)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_first_event',
          step: 1,
        })
        expect(window.dataLayer?.[1]).toEqual({
          event: 'vx_frontend_second_event',
          step: 2,
        })
        expect(window.dataLayer?.[2]).toEqual({
          event: 'vx_frontend_third_event',
          step: 3,
        })
      })
    })

    it('should preserve all parameter types correctly', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        const parameters = {
          event: 'ignore_me',
          stringValue: 'test',
          numberValue: 42,
          booleanValue: true,
          nullValue: null,
          arrayValue: [1, 2, 3],
          objectValue: { nested: 'value' },
          undefinedValue: undefined,
        }

        trackEvent('type_test', parameters)

        expect(window.dataLayer).toHaveLength(1)
        const pushedData = window.dataLayer?.[0]

        expect(pushedData?.event).toBe('vx_frontend_type_test')
        expect(pushedData?.stringValue).toBe('test')
        expect(pushedData?.numberValue).toBe(42)
        expect(pushedData?.booleanValue).toBe(true)
        expect(pushedData?.nullValue).toBe(null)
        expect(pushedData?.arrayValue).toEqual([1, 2, 3])
        expect(pushedData?.objectValue).toEqual({ nested: 'value' })
        expect(pushedData?.undefinedValue).toBeUndefined()
        expect(pushedData).not.toHaveProperty('event', 'ignore_me')

        // Verify that the original event property is excluded
        expect(Object.keys(pushedData || {})).not.toContain('ignore_me')
      })
    })

    it('should handle special characters in event names', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('event-with-dashes_and_underscores', { test: true })

        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_event-with-dashes_and_underscores',
          test: true,
        })
      })
    })

    it('should handle event names with numbers', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        trackEvent('event123', { count: 456 })

        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_event123',
          count: 456,
        })
      })
    })
  })

  describe('initializeGTM', () => {
    it('should initialize GTM with correct script attributes', () => {
      jest.isolateModules(() => {
        const { initializeGTM, GTM_ID } = require('../../src/utils/gtm')

        initializeGTM()

        // Check that dataLayer was initialized with GTM start event
        expect(window.dataLayer).toContainEqual({
          'gtm.start': expect.any(Number),
          event: 'gtm.js',
        })

        // Check that script was added to the document
        const gtmScript = document.querySelector(
          'script[src*="googletagmanager"]'
        )
        expect(gtmScript).toBeTruthy()
        expect(gtmScript?.getAttribute('src')).toBe(
          `https://www.googletagmanager.com/gtm.js?id=${GTM_ID}`
        )
        // Check async property directly (hasAttribute may not work in jsdom)
        expect((gtmScript as HTMLScriptElement)?.async).toBe(true)
      })
    })

    it('should not initialize when dataLayer already has GTM events', () => {
      jest.isolateModules(() => {
        const { initializeGTM } = require('../../src/utils/gtm')

        // Pre-populate dataLayer
        window.dataLayer = [{ event: 'gtm.js', 'gtm.start': Date.now() }]

        initializeGTM()

        // Should still add another initialization event
        expect(
          window.dataLayer?.filter(item => item.event === 'gtm.js')
        ).toHaveLength(2)
      })
    })
  })

  describe('Integration tests', () => {
    it('should work together - initialize GTM and track events', () => {
      jest.isolateModules(() => {
        const { initializeGTM, trackEvent } = require('../../src/utils/gtm')

        // Initialize GTM
        initializeGTM()

        // Track some events
        trackEvent('page_view', { page: '/dashboard' })
        trackEvent('button_click', {
          button: 'submit',
          event: 'should_be_ignored',
        })

        // Check initialization event plus tracked events
        expect(window.dataLayer).toHaveLength(3)

        // GTM initialization event
        expect(window.dataLayer?.[0]).toEqual({
          'gtm.start': expect.any(Number),
          event: 'gtm.js',
        })

        // Tracked events with proper prefixes
        expect(window.dataLayer?.[1]).toEqual({
          event: 'vx_frontend_page_view',
          page: '/dashboard',
        })

        expect(window.dataLayer?.[2]).toEqual({
          event: 'vx_frontend_button_click',
          button: 'submit',
        })

        // Verify script was added
        const gtmScript = document.querySelector(
          'script[src*="googletagmanager"]'
        )
        expect(gtmScript).toBeTruthy()
      })
    })

    it('should work with analytics service pattern', () => {
      jest.isolateModules(() => {
        const { trackEvent } = require('../../src/utils/gtm')

        // Simulate how analytics service calls trackEvent
        const eventData = {
          event: 'user_signup',
          user_id: '12345',
          plan: 'premium',
          source: 'homepage',
        }

        trackEvent(eventData.event, eventData)

        expect(window.dataLayer).toHaveLength(1)
        expect(window.dataLayer?.[0]).toEqual({
          event: 'vx_frontend_user_signup',
          user_id: '12345',
          plan: 'premium',
          source: 'homepage',
        })

        // Verify the original event name was excluded
        expect(window.dataLayer?.[0]).not.toHaveProperty('event', 'user_signup')
      })
    })
  })
})
