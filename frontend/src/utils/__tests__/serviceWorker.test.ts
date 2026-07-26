import { cleanupLegacyServiceWorkers } from '../serviceWorker'

// The behaviour under test changed in #688: the app used to KEEP
// `firebase-messaging-sw.js` registered for web push. Firebase is gone, VibeXP
// now ships no service worker at all, so this must unregister EVERY worker it
// finds — including the retired Firebase one.

interface Registration {
  unregister: jest.Mock
  active: { scriptURL: string }
}

const makeRegistration = (scriptURL: string): Registration => ({
  unregister: jest.fn().mockResolvedValue(true),
  active: { scriptURL },
})

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

describe('cleanupLegacyServiceWorkers', () => {
  const originalNavigator = globalThis.navigator
  let getRegistrations: jest.Mock
  let cachesKeys: jest.Mock
  let cachesDelete: jest.Mock

  beforeEach(() => {
    jest.clearAllMocks()

    getRegistrations = jest.fn().mockResolvedValue([])
    Object.defineProperty(globalThis, 'navigator', {
      value: { serviceWorker: { getRegistrations } },
      configurable: true,
      writable: true,
    })

    cachesKeys = jest.fn().mockResolvedValue([])
    cachesDelete = jest.fn().mockResolvedValue(true)
    Object.defineProperty(window, 'caches', {
      value: { keys: cachesKeys, delete: cachesDelete },
      configurable: true,
      writable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: originalNavigator,
      configurable: true,
      writable: true,
    })
    // @ts-expect-error — test cleanup of an optional DOM global
    delete window.caches
  })

  it('unregisters every service worker, including the retired Firebase one', async () => {
    const firebase = makeRegistration(
      'https://app.test/firebase-messaging-sw.js'
    )
    const legacy = makeRegistration('https://app.test/dev-sw.js')
    getRegistrations.mockResolvedValue([firebase, legacy])

    cleanupLegacyServiceWorkers()
    await flush()

    expect(firebase.unregister).toHaveBeenCalledTimes(1)
    expect(legacy.unregister).toHaveBeenCalledTimes(1)
  })

  it('deletes legacy caches by exact name and by prefix, leaving others alone', async () => {
    cachesKeys.mockResolvedValue([
      'api-cache', // exact legacy name
      'avatar-cache', // exact legacy name
      'workbox-precache-v2', // legacy prefix
      'some-unrelated-cache', // must survive
    ])

    cleanupLegacyServiceWorkers()
    await flush()

    expect(cachesDelete).toHaveBeenCalledWith('api-cache')
    expect(cachesDelete).toHaveBeenCalledWith('avatar-cache')
    expect(cachesDelete).toHaveBeenCalledWith('workbox-precache-v2')
    expect(cachesDelete).not.toHaveBeenCalledWith('some-unrelated-cache')
    expect(cachesDelete).toHaveBeenCalledTimes(3)
  })

  it('is a no-op when the browser has no serviceWorker support', async () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: {},
      configurable: true,
      writable: true,
    })

    expect(() => {
      cleanupLegacyServiceWorkers()
    }).not.toThrow()
    await flush()

    expect(getRegistrations).not.toHaveBeenCalled()
  })

  it('swallows a rejected getRegistrations so startup is never blocked', async () => {
    getRegistrations.mockRejectedValue(new Error('denied'))

    expect(() => {
      cleanupLegacyServiceWorkers()
    }).not.toThrow()
    await flush()
  })

  it('swallows a rejected caches.keys so startup is never blocked', async () => {
    cachesKeys.mockRejectedValue(new Error('denied'))

    expect(() => {
      cleanupLegacyServiceWorkers()
    }).not.toThrow()
    await flush()

    expect(cachesDelete).not.toHaveBeenCalled()
  })
})
