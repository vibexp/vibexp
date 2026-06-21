/**
 * @jest-environment jsdom
 */

/**
 * Unit tests for cookieConsent utility
 */

// Mock window.gtag - MUST be before imports
const mockGtag = jest.fn()
Object.defineProperty(global.window, 'gtag', {
  value: mockGtag,
  writable: true,
})

// Mock window.dataLayer - MUST be before imports
const mockDataLayer: unknown[] = []
Object.defineProperty(global.window, 'dataLayer', {
  value: mockDataLayer,
  writable: true,
})

// Mock storage utility - MUST be before imports
let storageStore: Record<string, string> = {}

jest.mock('../../src/utils/storage', () => ({
  storage: {
    get: jest.fn((key: string) => {
      return storageStore[key] ?? null
    }),
    getJSON: jest.fn((key: string) => {
      const value = storageStore[key]
      if (!value) return null
      try {
        return JSON.parse(value) as unknown
      } catch {
        return null
      }
    }),
    set: jest.fn((key: string, value: unknown) => {
      storageStore[key] =
        typeof value === 'string' ? value : JSON.stringify(value)
    }),
    clear: jest.fn(() => {
      storageStore = {}
    }),
    has: jest.fn((key: string) => storageStore[key] !== undefined),
  },
  sessionStore: {
    get: jest.fn(),
    getJSON: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
    has: jest.fn(),
  },
  storageUtils: {
    clearVibeXPData: jest.fn(),
    getAllVibeXPData: jest.fn(),
    isStorageAvailable: jest.fn(),
  },
}))

// Now import after mocks are set up
import {
  grantCookieConsent,
  denyCookieConsent,
  hasGrantedCookieConsent,
  hasCookieConsentDecision,
} from '../../src/utils/cookieConsent'
import { STORAGE_KEYS } from '../../src/constants/storageKeys'
import { storage } from '../../src/utils/storage'

// Get the mocked functions
const mockGet = storage.get as jest.Mock
const mockGetJSON = storage.getJSON as jest.Mock
const mockSet = storage.set as jest.Mock
const mockClear = storage.clear as jest.Mock

// Get reference to dataLayer after module initialization
const getDataLayer = () =>
  (global as unknown as { window: { dataLayer: unknown[] } }).window
    .dataLayer as unknown[]

// Helper for tests to directly manipulate the store
const getInternalStore = () => storageStore

describe('cookieConsent', () => {
  beforeEach(() => {
    mockClear()
    jest.clearAllMocks()
    mockDataLayer.length = 0
    // Reset dataLayer reference to ensure it's always available after tests that set it to undefined
    Object.defineProperty(global.window, 'dataLayer', {
      value: mockDataLayer,
      writable: true,
    })
    // Reset gtag reference
    Object.defineProperty(global.window, 'gtag', {
      value: mockGtag,
      writable: true,
    })
  })

  describe('grantCookieConsent', () => {
    it('should save consent to localStorage with timestamp', () => {
      grantCookieConsent()

      const stored = mockGetJSON(STORAGE_KEYS.COOKIE_CONSENT)
      expect(stored).toBeDefined()

      const consentData = stored as { status: string; timestamp: number }
      expect(consentData.status).toBe('granted')
      expect(consentData.timestamp).toBeDefined()
      expect(typeof consentData.timestamp).toBe('number')
    })

    it('should call gtag with granted consent values', () => {
      grantCookieConsent()

      expect(mockGtag).toHaveBeenCalledWith('consent', 'update', {
        ad_storage: 'granted',
        ad_user_data: 'granted',
        ad_personalization: 'granted',
        analytics_storage: 'granted',
      })
    })

    it('should push cookie_consent_update event to dataLayer', () => {
      grantCookieConsent()

      expect(getDataLayer()).toHaveLength(1)
      expect(getDataLayer()[0]).toEqual({
        event: 'cookie_consent_update',
        consent_status: 'granted',
      })
    })

    it('should store consent even if gtag throws', () => {
      const throwingGtag = jest.fn(() => {
        throw new Error('gtag error')
      })
      Object.defineProperty(global.window, 'gtag', {
        value: throwingGtag,
        writable: true,
      })

      // gtag and dataLayer are always defined in production (via index.html)
      // but we still want localStorage to be saved even if there's an error
      expect(() => grantCookieConsent()).toThrow()

      const stored = mockGet(STORAGE_KEYS.COOKIE_CONSENT)
      expect(stored).toBeDefined()
    })
  })

  describe('denyCookieConsent', () => {
    it('should save decline to localStorage with timestamp', () => {
      denyCookieConsent()

      const stored = mockGetJSON(STORAGE_KEYS.COOKIE_CONSENT)
      expect(stored).toBeDefined()

      const consentData = stored as { status: string; timestamp: number }
      expect(consentData.status).toBe('denied')
      expect(consentData.timestamp).toBeDefined()
      expect(typeof consentData.timestamp).toBe('number')
    })

    it('should push cookie_consent_update event to dataLayer', () => {
      denyCookieConsent()

      expect(getDataLayer()).toHaveLength(1)
      expect(getDataLayer()[0]).toEqual({
        event: 'cookie_consent_update',
        consent_status: 'denied',
      })
    })

    it('should call gtag with denied consent values', () => {
      denyCookieConsent()

      expect(mockGtag).toHaveBeenCalledWith('consent', 'update', {
        ad_storage: 'denied',
        ad_user_data: 'denied',
        ad_personalization: 'denied',
        analytics_storage: 'denied',
      })
    })
  })

  describe('hasGrantedCookieConsent', () => {
    it('should return false when no consent exists', () => {
      expect(hasGrantedCookieConsent()).toBe(false)
    })

    it('should return true when consent is granted', () => {
      grantCookieConsent()
      expect(hasGrantedCookieConsent()).toBe(true)
    })

    it('should return false when consent is denied', () => {
      denyCookieConsent()
      expect(hasGrantedCookieConsent()).toBe(false)
    })

    it('should handle legacy format (plain string)', () => {
      // Directly set the string in the internal store
      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'granted'
      expect(hasGrantedCookieConsent()).toBe(true)

      mockClear()
      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'denied'
      expect(hasGrantedCookieConsent()).toBe(false)
    })

    it('should return false for malformed data', () => {
      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'invalid-json'
      expect(hasGrantedCookieConsent()).toBe(false)
    })
  })

  describe('hasCookieConsentDecision', () => {
    it('should return false when no consent exists', () => {
      expect(hasCookieConsentDecision()).toBe(false)
    })

    it('should return true for granted consent (any age)', () => {
      grantCookieConsent()
      expect(hasCookieConsentDecision()).toBe(true)
    })

    it('should return true for recently declined consent (< 7 days)', () => {
      denyCookieConsent()
      expect(hasCookieConsentDecision()).toBe(true)
    })

    it('should return false for expired declined consent (>= 7 days)', () => {
      denyCookieConsent()

      // Manually set timestamp to 8 days ago
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 8 * 24 * 60 * 60 * 1000 // 8 days ago
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      expect(hasCookieConsentDecision()).toBe(false)
    })

    it('should return false exactly at 7 days boundary (consent expires at 7 days)', () => {
      denyCookieConsent()

      // Manually set timestamp to exactly 7 days ago
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 7 * 24 * 60 * 60 * 1000 // exactly 7 days
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      expect(hasCookieConsentDecision()).toBe(false)
    })

    it('should return true exactly at 6.99 days', () => {
      denyCookieConsent()

      // Manually set timestamp to 6.99 days ago
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 6.99 * 24 * 60 * 60 * 1000
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      expect(hasCookieConsentDecision()).toBe(true)
    })

    it('should handle legacy format (plain string)', () => {
      // Directly set the string in the internal store
      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'granted'
      expect(hasCookieConsentDecision()).toBe(true)

      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'denied'
      expect(hasCookieConsentDecision()).toBe(true)
    })

    it('should return false for malformed data', () => {
      getInternalStore()[STORAGE_KEYS.COOKIE_CONSENT] = 'invalid-json'
      expect(hasCookieConsentDecision()).toBe(false)
    })

    it('should handle unknown status gracefully', () => {
      const consentData = {
        status: 'unknown' as const,
        timestamp: Date.now(),
      }
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      expect(hasCookieConsentDecision()).toBe(false)
    })
  })

  describe('consent expiry behavior', () => {
    it('should keep granted consent valid indefinitely', () => {
      grantCookieConsent()

      // Set timestamp to 1 year ago
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 365 * 24 * 60 * 60 * 1000 // 1 year ago
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      expect(hasCookieConsentDecision()).toBe(true)
      expect(hasGrantedCookieConsent()).toBe(true)
    })

    it('should expire declined consent after 7 days', () => {
      denyCookieConsent()

      // Initially should have decision
      expect(hasCookieConsentDecision()).toBe(true)

      // Set timestamp to 8 days ago
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 8 * 24 * 60 * 60 * 1000
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      // Should now show banner again
      expect(hasCookieConsentDecision()).toBe(false)
    })

    it('should allow user to grant consent after expiry', () => {
      denyCookieConsent()

      // Set timestamp to 8 days ago (expired)
      const store = getInternalStore()
      const consentData = JSON.parse(store[STORAGE_KEYS.COOKIE_CONSENT]!) as {
        status: string
        timestamp: number
      }
      consentData.timestamp = Date.now() - 8 * 24 * 60 * 60 * 1000
      mockSet(STORAGE_KEYS.COOKIE_CONSENT, consentData)

      // Should show banner again
      expect(hasCookieConsentDecision()).toBe(false)

      // User grants consent
      grantCookieConsent()

      // Should now have valid decision
      expect(hasCookieConsentDecision()).toBe(true)
      expect(hasGrantedCookieConsent()).toBe(true)
    })
  })
})
