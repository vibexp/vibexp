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

// Mock the generated client; unwrap stays real so the test exercises the same
// success/error resolution production uses.
const mockGeneratedClient = {
  POST: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('@/lib/apiClientGenerated', () => {
  const actual = jest.requireActual<typeof import('@/lib/apiClientGenerated')>(
    '@/lib/apiClientGenerated'
  )
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

const okResponse = { ok: true, status: 201, statusText: 'Created' } as Response
const success = () => ({ data: undefined, response: okResponse })

// Mock toast
const mockToast = {
  info: jest.fn(),
}

jest.mock('@/lib/toast', () => ({
  toast: mockToast,
}))

// fcm.ts logs failures via console.error now that no telemetry is bundled.
// Spy on it so we can assert error logging without noisy test output.
const consoleErrorSpy = jest
  .spyOn(console, 'error')
  .mockImplementation(() => {})

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
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
    })

    it('returns false when permission is default (not yet decided)', async () => {
      global.Notification = {
        requestPermission: jest.fn().mockResolvedValue('default'),
        permission: 'default',
      } as unknown as typeof Notification

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
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
      mockGeneratedClient.POST.mockResolvedValue(success())

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(true)
      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/device-tokens',
        {
          body: {
            token: 'fcm-token-abc',
            platform: 'web',
            user_agent: expect.any(String),
          },
        }
      )
    })

    it('returns false when getToken returns empty string', async () => {
      mockGetToken.mockResolvedValue('')

      const result = await fcmService.requestPermissionAndRegister()

      expect(result).toBe(false)
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
    })

    it('registers onMessage foreground handler after successful registration', async () => {
      mockGetToken.mockResolvedValue('fcm-token-xyz')
      mockGeneratedClient.POST.mockResolvedValue(success())

      await fcmService.requestPermissionAndRegister()

      expect(mockOnMessage).toHaveBeenCalledWith(
        mockMessaging,
        expect.any(Function)
      )
    })

    it('reuses existing Firebase app if already initialized', async () => {
      mockGetApps.mockReturnValue([mockApp])
      mockGetToken.mockResolvedValue('fcm-token-reuse')
      mockGeneratedClient.POST.mockResolvedValue(success())

      await fcmService.requestPermissionAndRegister()

      expect(mockInitializeApp).not.toHaveBeenCalled()
      expect(mockGetMessaging).toHaveBeenCalledWith(mockApp)
    })

    it('uses vapid key from getFirebaseVapidKey', async () => {
      mockGetToken.mockResolvedValue('fcm-token-vapid')
      mockGetFirebaseVapidKey.mockReturnValue('custom-vapid-key')
      mockGeneratedClient.POST.mockResolvedValue(success())

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

    it('resolves to false and logs the error when getToken throws AbortError', async () => {
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
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[FCM] requestPermissionAndRegister failed:',
        abortError
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
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
      expect(consoleErrorSpy).toHaveBeenCalled()
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
      expect(mockGeneratedClient.POST).not.toHaveBeenCalled()
      expect(consoleErrorSpy).toHaveBeenCalled()
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
      mockGeneratedClient.DELETE.mockResolvedValue({
        data: undefined,
        response: { ok: true, status: 204, statusText: 'No Content' },
      })

      await fcmService.revokeToken()

      expect(mockDeleteToken).toHaveBeenCalledWith(mockMessaging)
      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/device-tokens',
        {
          body: { token: 'fcm-token-to-revoke' },
        }
      )
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
      expect(mockGeneratedClient.DELETE).not.toHaveBeenCalled()
    })
  })
})
