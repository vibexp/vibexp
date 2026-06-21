// ---------------------------------------------------------------------------
// Jest automatically uses tests/mocks/firebase-app.ts and
// tests/mocks/firebase-messaging.ts for these packages (moduleNameMapper).
// Explicitly calling jest.mock still works and lets us control return values.
// ---------------------------------------------------------------------------

const mockInitializeApp = jest.fn()
const mockGetApps = jest.fn()
const mockGetMessaging = jest.fn()
const mockGetToken = jest.fn()
const mockDeleteToken = jest.fn()
const mockOnMessage = jest.fn()

jest.mock('firebase/app', () => ({
  initializeApp: mockInitializeApp,
  getApps: mockGetApps,
}))

jest.mock('firebase/messaging', () => ({
  getMessaging: mockGetMessaging,
  getToken: mockGetToken,
  deleteToken: mockDeleteToken,
  onMessage: mockOnMessage,
}))

// ---------------------------------------------------------------------------
// @/lib/firebaseEnv is mapped to tests/mocks/firebaseEnv.ts via moduleNameMapper.
// Mock it here so we can control isFirebaseConfigured() return values.
// ---------------------------------------------------------------------------

const mockGetFirebaseConfig = jest.fn()
const mockGetFirebaseVapidKey = jest.fn()
const mockIsFirebaseConfigured = jest.fn()

jest.mock('@/lib/firebaseEnv', () => ({
  getFirebaseConfig: mockGetFirebaseConfig,
  getFirebaseVapidKey: mockGetFirebaseVapidKey,
  isFirebaseConfigured: mockIsFirebaseConfigured,
}))

// Mock apiClient
const mockApiClient = {
  post: jest.fn(),
  delete: jest.fn(),
}

jest.mock('@/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Mock toast
const mockToast = {
  info: jest.fn(),
}

jest.mock('@/lib/toast', () => ({
  toast: mockToast,
}))

// Mock the handled Sentry capture helper so we can assert error reporting
// without hitting the real Sentry SDK.
const mockCaptureException = jest.fn()

jest.mock('@/utils/sentry', () => ({
  captureException: mockCaptureException,
}))

import { fcmService } from '../fcm'

const mockMessaging = { name: 'mock-messaging' }
const mockApp = { name: 'mock-app' }

describe('FCMService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockGetApps.mockReturnValue([])
    mockInitializeApp.mockReturnValue(mockApp)
    mockGetMessaging.mockReturnValue(mockMessaging)
    mockOnMessage.mockReturnValue(jest.fn())
    mockGetFirebaseConfig.mockReturnValue({ apiKey: 'test', appId: 'test' })
    mockGetFirebaseVapidKey.mockReturnValue('test-vapid-key')
    mockIsFirebaseConfigured.mockReturnValue(false)
    // Reset instance state between tests
    ;(fcmService as unknown as { messagingInstance: null }).messagingInstance =
      null
  })

  // ---------------------------------------------------------------------------
  // isFCMConfigured
  // ---------------------------------------------------------------------------

  describe('isFCMConfigured', () => {
    it('delegates to isFirebaseConfigured and returns false when unconfigured', () => {
      mockIsFirebaseConfigured.mockReturnValue(false)
      expect(fcmService.isFCMConfigured()).toBe(false)
    })

    it('delegates to isFirebaseConfigured and returns true when configured', () => {
      mockIsFirebaseConfigured.mockReturnValue(true)
      expect(fcmService.isFCMConfigured()).toBe(true)
    })
  })

  // ---------------------------------------------------------------------------
  // requestPermissionAndRegister — permission denied
  // ---------------------------------------------------------------------------

  describe('requestPermissionAndRegister - permission denied', () => {
    it('returns false and skips registration when permission is denied', async () => {
      global.Notification = {
        requestPermission: jest.fn().mockResolvedValue('denied'),
        permission: 'denied',
      } as unknown as typeof Notification

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockGetToken).not.toHaveBeenCalled()
      expect(mockApiClient.post).not.toHaveBeenCalled()
    })

    it('returns false when permission is default (not yet decided)', async () => {
      global.Notification = {
        requestPermission: jest.fn().mockResolvedValue('default'),
        permission: 'default',
      } as unknown as typeof Notification

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockApiClient.post).not.toHaveBeenCalled()
    })
  })

  // ---------------------------------------------------------------------------
  // requestPermissionAndRegister — permission granted, happy path
  // ---------------------------------------------------------------------------

  describe('requestPermissionAndRegister - permission granted', () => {
    const mockSWRegistration = {
      scope: '/',
      active: { state: 'activated' },
    } as unknown as ServiceWorkerRegistration

    beforeEach(() => {
      global.Notification = {
        requestPermission: jest.fn().mockResolvedValue('granted'),
        permission: 'granted',
      } as unknown as typeof Notification

      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          register: jest.fn().mockResolvedValue(mockSWRegistration),
          getRegistration: jest.fn().mockResolvedValue(mockSWRegistration),
          ready: Promise.resolve(mockSWRegistration),
        },
        writable: true,
        configurable: true,
      })
    })

    it('returns true and posts device token when permission is granted', async () => {
      mockGetToken.mockResolvedValue('fcm-token-abc')
      mockApiClient.post.mockResolvedValue(undefined)

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(true)
      expect(mockApiClient.post).toHaveBeenCalledWith('/device-tokens', {
        token: 'fcm-token-abc',
        platform: 'web',
        user_agent: expect.any(String),
      })
    })

    it('returns false when getToken returns empty string', async () => {
      mockGetToken.mockResolvedValue('')

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockApiClient.post).not.toHaveBeenCalled()
    })

    it('registers onMessage foreground handler after successful registration', async () => {
      mockGetToken.mockResolvedValue('fcm-token-xyz')
      mockApiClient.post.mockResolvedValue(undefined)

      await fcmService.requestPermissionAndRegister()

      expect(mockOnMessage).toHaveBeenCalledWith(
        mockMessaging,
        expect.any(Function)
      )
    })

    it('reuses existing Firebase app if already initialized', async () => {
      mockGetApps.mockReturnValue([mockApp])
      mockGetToken.mockResolvedValue('fcm-token-reuse')
      mockApiClient.post.mockResolvedValue(undefined)

      await fcmService.requestPermissionAndRegister()

      expect(mockInitializeApp).not.toHaveBeenCalled()
      expect(mockGetMessaging).toHaveBeenCalledWith(mockApp)
    })

    it('uses vapid key from getFirebaseVapidKey', async () => {
      mockGetToken.mockResolvedValue('fcm-token-vapid')
      mockGetFirebaseVapidKey.mockReturnValue('custom-vapid-key')
      mockApiClient.post.mockResolvedValue(undefined)

      await fcmService.requestPermissionAndRegister()

      expect(mockGetToken).toHaveBeenCalledWith(
        mockMessaging,
        expect.objectContaining({ vapidKey: 'custom-vapid-key' })
      )
    })
  })

  // ---------------------------------------------------------------------------
  // requestPermissionAndRegister — registration failures (AbortError race etc.)
  // ---------------------------------------------------------------------------

  describe('requestPermissionAndRegister - registration failures', () => {
    const mockSWRegistration = {
      scope: '/',
      active: { state: 'activated' },
    } as unknown as ServiceWorkerRegistration

    beforeEach(() => {
      global.Notification = {
        requestPermission: jest.fn().mockResolvedValue('granted'),
        permission: 'granted',
      } as unknown as typeof Notification
    })

    it('resolves to false and reports to Sentry when getToken throws AbortError', async () => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          register: jest.fn().mockResolvedValue(mockSWRegistration),
          ready: Promise.resolve(mockSWRegistration),
        },
        writable: true,
        configurable: true,
      })

      const abortError = new Error('AbortError: no active Service Worker')
      abortError.name = 'AbortError'
      mockGetToken.mockRejectedValue(abortError)

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockApiClient.post).not.toHaveBeenCalled()
      expect(mockCaptureException).toHaveBeenCalledWith(
        abortError,
        expect.objectContaining({
          operation: 'requestPermissionAndRegister',
        })
      )
    })

    it('resolves to false when serviceWorker.ready rejects', async () => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          register: jest.fn().mockResolvedValue(mockSWRegistration),
          ready: Promise.reject(new Error('SW activation failed')),
        },
        writable: true,
        configurable: true,
      })

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockGetToken).not.toHaveBeenCalled()
      expect(mockApiClient.post).not.toHaveBeenCalled()
      expect(mockCaptureException).toHaveBeenCalled()
    })

    it('resolves to false when navigator.serviceWorker is unavailable', async () => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: undefined,
        writable: true,
        configurable: true,
      })

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockGetToken).not.toHaveBeenCalled()
      expect(mockApiClient.post).not.toHaveBeenCalled()
      expect(mockCaptureException).toHaveBeenCalled()
    })
  })

  // ---------------------------------------------------------------------------
  // revokeToken
  // ---------------------------------------------------------------------------

  describe('revokeToken', () => {
    const mockSWRegistration = {
      scope: '/',
      active: { state: 'activated' },
    } as unknown as ServiceWorkerRegistration

    beforeEach(() => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          register: jest.fn().mockResolvedValue(mockSWRegistration),
          getRegistration: jest.fn().mockResolvedValue(mockSWRegistration),
        },
        writable: true,
        configurable: true,
      })
    })

    it('calls deleteToken and removes device token from backend', async () => {
      mockGetToken.mockResolvedValue('fcm-token-to-revoke')
      mockDeleteToken.mockResolvedValue(true)
      mockApiClient.delete.mockResolvedValue(undefined)

      await fcmService.revokeToken()

      expect(mockDeleteToken).toHaveBeenCalledWith(mockMessaging)
      expect(mockApiClient.delete).toHaveBeenCalledWith('/device-tokens', {
        token: 'fcm-token-to-revoke',
      })
    })

    it('does not throw if getRegistration returns undefined', async () => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          getRegistration: jest.fn().mockResolvedValue(undefined),
        },
        writable: true,
        configurable: true,
      })

      await expect(fcmService.revokeToken()).resolves.toBeUndefined()
      expect(mockDeleteToken).not.toHaveBeenCalled()
    })

    it('silently ignores errors during cleanup', async () => {
      Object.defineProperty(global.navigator, 'serviceWorker', {
        value: {
          getRegistration: jest
            .fn()
            .mockRejectedValue(new Error('SW not available')),
        },
        writable: true,
        configurable: true,
      })

      await expect(fcmService.revokeToken()).resolves.toBeUndefined()
    })

    it('skips backend call when token is empty', async () => {
      mockGetToken.mockResolvedValue('')

      await fcmService.revokeToken()

      expect(mockDeleteToken).not.toHaveBeenCalled()
      expect(mockApiClient.delete).not.toHaveBeenCalled()
    })
  })
})
