/**
 * @jest-environment jsdom
 */

/**
 * Unit tests for storage utilities
 */

import {
  LEGACY_STORAGE_KEYS,
  STORAGE_KEYS,
} from '../../src/constants/storageKeys'
import { sessionStore, storage, storageUtils } from '../../src/utils/storage'

describe('storage utilities', () => {
  beforeEach(() => {
    // Restore mocks first to ensure storage operations work
    jest.restoreAllMocks()
    // Clear all storage before each test
    localStorage.clear()
    sessionStorage.clear()
  })

  describe('storage (localStorage wrapper)', () => {
    describe('get', () => {
      it('returns null for non-existent key', () => {
        expect(storage.get(STORAGE_KEYS.CURRENT_TEAM_ID)).toBeNull()
      })

      it('returns raw string value', () => {
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'test-token')
        expect(storage.get(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe('test-token')
      })

      it('returns JSON string as-is (not parsed)', () => {
        const jsonString = JSON.stringify({ foo: 'bar' })
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, jsonString)
        expect(storage.get(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(jsonString)
      })

      it('handles storage access errors gracefully', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output in tests
          })
        jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
          throw new Error('Storage access denied')
        })

        expect(storage.get(STORAGE_KEYS.CURRENT_TEAM_ID)).toBeNull()
        expect(consoleSpy).toHaveBeenCalled()
      })
    })

    describe('getJSON', () => {
      it('returns null for non-existent key', () => {
        expect(
          storage.getJSON<{ status: string }>(STORAGE_KEYS.COOKIE_CONSENT)
        ).toBeNull()
      })

      it('parses JSON string and returns typed object', () => {
        const data = { status: 'granted', timestamp: 12345 }
        localStorage.setItem(STORAGE_KEYS.COOKIE_CONSENT, JSON.stringify(data))

        const result = storage.getJSON<{ status: string; timestamp: number }>(
          STORAGE_KEYS.COOKIE_CONSENT
        )
        expect(result).toEqual(data)
      })

      it('returns null for invalid JSON', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        localStorage.setItem(STORAGE_KEYS.COOKIE_CONSENT, 'not-valid-json')

        expect(
          storage.getJSON<{ status: string }>(STORAGE_KEYS.COOKIE_CONSENT)
        ).toBeNull()
        expect(consoleSpy).toHaveBeenCalled()
      })

      it('handles storage access errors gracefully', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
          throw new Error('Storage access denied')
        })

        expect(
          storage.getJSON<{ status: string }>(STORAGE_KEYS.COOKIE_CONSENT)
        ).toBeNull()
        expect(consoleSpy).toHaveBeenCalled()
      })
    })

    describe('set', () => {
      it('stores string value directly', () => {
        storage.set(STORAGE_KEYS.CURRENT_TEAM_ID, 'my-token')
        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(
          'my-token'
        )
      })

      it('serializes non-string values to JSON', () => {
        const data = { status: 'granted', timestamp: 12345 }
        storage.set(STORAGE_KEYS.COOKIE_CONSENT, data)
        expect(localStorage.getItem(STORAGE_KEYS.COOKIE_CONSENT)).toBe(
          JSON.stringify(data)
        )
      })

      it('handles QuotaExceededError', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        const quotaError = new Error('Quota exceeded')
        quotaError.name = 'QuotaExceededError'
        jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
          throw quotaError
        })

        storage.set(STORAGE_KEYS.CURRENT_TEAM_ID, 'test')
        expect(consoleSpy).toHaveBeenCalledWith(
          expect.stringContaining('quota exceeded')
        )
      })

      it('handles other storage errors', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
          throw new Error('Security error')
        })

        storage.set(STORAGE_KEYS.CURRENT_TEAM_ID, 'test')
        expect(consoleSpy).toHaveBeenCalled()
      })
    })

    describe('remove', () => {
      it('removes existing key', () => {
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'test')
        storage.remove(STORAGE_KEYS.CURRENT_TEAM_ID)
        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBeNull()
      })

      it('handles removal errors gracefully', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        jest.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
          throw new Error('Storage error')
        })

        storage.remove(STORAGE_KEYS.CURRENT_TEAM_ID)
        expect(consoleSpy).toHaveBeenCalled()
      })
    })

    describe('has', () => {
      it('returns false for non-existent key', () => {
        expect(storage.has(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(false)
      })

      it('returns true for existing key', () => {
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'test')
        expect(storage.has(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(true)
      })

      it('handles errors gracefully', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
          throw new Error('Storage error')
        })

        expect(storage.has(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(false)
        expect(consoleSpy).toHaveBeenCalled()
      })
    })

    describe('clear', () => {
      it('clears all items from localStorage', () => {
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'test1')
        localStorage.setItem(STORAGE_KEYS.COOKIE_CONSENT, 'test2')
        storage.clear()
        expect(localStorage.length).toBe(0)
      })

      it('handles clear errors gracefully', () => {
        const consoleSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {
            // Suppress error output
          })
        jest.spyOn(Storage.prototype, 'clear').mockImplementation(() => {
          throw new Error('Storage error')
        })

        storage.clear()
        expect(consoleSpy).toHaveBeenCalled()
      })
    })
  })

  describe('sessionStore (sessionStorage wrapper)', () => {
    it('works with sessionStorage', () => {
      sessionStore.set(STORAGE_KEYS.ANALYTICS_REFERRER, '/page')
      expect(sessionStorage.getItem(STORAGE_KEYS.ANALYTICS_REFERRER)).toBe(
        '/page'
      )
      expect(sessionStore.get(STORAGE_KEYS.ANALYTICS_REFERRER)).toBe('/page')
    })

    it('has all the same methods as storage', () => {
      expect(typeof sessionStore.get).toBe('function')
      expect(typeof sessionStore.getJSON).toBe('function')
      expect(typeof sessionStore.set).toBe('function')
      expect(typeof sessionStore.remove).toBe('function')
      expect(typeof sessionStore.has).toBe('function')
      expect(typeof sessionStore.clear).toBe('function')
    })
  })

  describe('storageUtils', () => {
    describe('clearVibeXPData', () => {
      it('clears all VibeXP keys from localStorage and sessionStorage', () => {
        // Set up data in both storages
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'team-123')
        localStorage.setItem(STORAGE_KEYS.COOKIE_CONSENT, 'consent')
        sessionStorage.setItem(STORAGE_KEYS.ANALYTICS_REFERRER, '/page')
        sessionStorage.setItem(STORAGE_KEYS.PURCHASE_TRACKED, 'true')
        // Also add a non-VibeXP key that should be preserved
        localStorage.setItem('other_app_key', 'preserved')

        storageUtils.clearVibeXPData()

        // VibeXP keys should be removed
        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBeNull()
        expect(localStorage.getItem(STORAGE_KEYS.COOKIE_CONSENT)).toBeNull()
        expect(
          sessionStorage.getItem(STORAGE_KEYS.ANALYTICS_REFERRER)
        ).toBeNull()
        expect(sessionStorage.getItem(STORAGE_KEYS.PURCHASE_TRACKED)).toBeNull()
        // Non-VibeXP key should be preserved
        expect(localStorage.getItem('other_app_key')).toBe('preserved')
      })
    })

    describe('getAllVibeXPData', () => {
      it('returns all VibeXP storage keys and their values', () => {
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'my-token')
        sessionStorage.setItem(STORAGE_KEYS.ANALYTICS_REFERRER, '/page')

        // Mock production environment to suppress warning
        const originalEnv = process.env.NODE_ENV
        process.env.NODE_ENV = 'production'

        const result = storageUtils.getAllVibeXPData()

        process.env.NODE_ENV = originalEnv

        expect(result[STORAGE_KEYS.CURRENT_TEAM_ID]).toEqual({
          local: 'my-token',
          session: null,
        })
        expect(result[STORAGE_KEYS.ANALYTICS_REFERRER]).toEqual({
          local: null,
          session: '/page',
        })
      })

      it('warns about sensitive data in development', () => {
        const consoleSpy = jest
          .spyOn(console, 'warn')
          .mockImplementation(() => {
            // Suppress warning
          })
        const originalEnv = process.env.NODE_ENV
        process.env.NODE_ENV = 'development'

        storageUtils.getAllVibeXPData()

        process.env.NODE_ENV = originalEnv

        expect(consoleSpy).toHaveBeenCalledWith(
          expect.stringContaining('sensitive data')
        )
      })
    })

    describe('isStorageAvailable', () => {
      it('returns true for both storages when available', () => {
        const result = storageUtils.isStorageAvailable()
        expect(result).toEqual({ local: true, session: true })
      })

      it('returns false for localStorage when unavailable', () => {
        // Mock Storage.prototype.setItem to throw only for test key
        const originalSetItem = Storage.prototype.setItem
        jest.spyOn(Storage.prototype, 'setItem').mockImplementation(function (
          this: Storage,
          key: string,
          value: string
        ) {
          if (key === '__storage_test__' && this === localStorage) {
            throw new Error('Storage disabled')
          }
          originalSetItem.call(this, key, value)
        })

        const result = storageUtils.isStorageAvailable()
        expect(result.local).toBe(false)
        expect(result.session).toBe(true)
      })
    })

    describe('migrateStorageKeys', () => {
      it('migrates legacy localStorage keys to new keys', () => {
        localStorage.setItem(
          LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID,
          'legacy-team-id'
        )
        localStorage.setItem(
          LEGACY_STORAGE_KEYS.COOKIE_CONSENT,
          JSON.stringify({ status: 'granted' })
        )

        storageUtils.migrateStorageKeys()

        // New keys should have the values
        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(
          'legacy-team-id'
        )
        expect(localStorage.getItem(STORAGE_KEYS.COOKIE_CONSENT)).toBe(
          JSON.stringify({ status: 'granted' })
        )

        // Legacy keys should be removed
        expect(
          localStorage.getItem(LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID)
        ).toBeNull()
        expect(
          localStorage.getItem(LEGACY_STORAGE_KEYS.COOKIE_CONSENT)
        ).toBeNull()
      })

      it('deletes any lingering auth tokens without re-creating them', () => {
        // Auth is now cookie-based: both the legacy `auth_token` and the old
        // `vx_auth_token` must be wiped (never migrated) on init.
        localStorage.setItem(LEGACY_STORAGE_KEYS.AUTH_TOKEN, 'legacy-jwt')
        localStorage.setItem('vx_auth_token', 'prefixed-jwt')

        storageUtils.migrateStorageKeys()

        expect(localStorage.getItem(LEGACY_STORAGE_KEYS.AUTH_TOKEN)).toBeNull()
        expect(localStorage.getItem('vx_auth_token')).toBeNull()
      })

      it('migrates legacy sessionStorage keys to new keys', () => {
        sessionStorage.setItem(
          LEGACY_STORAGE_KEYS.PENDING_INVITATION_TOKEN,
          'legacy-invite'
        )
        sessionStorage.setItem(
          LEGACY_STORAGE_KEYS.ANALYTICS_REFERRER,
          '/legacy-page'
        )

        storageUtils.migrateStorageKeys()

        // New keys should have the values
        expect(
          sessionStorage.getItem(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
        ).toBe('legacy-invite')
        expect(sessionStorage.getItem(STORAGE_KEYS.ANALYTICS_REFERRER)).toBe(
          '/legacy-page'
        )

        // Legacy keys should be removed
        expect(
          sessionStorage.getItem(LEGACY_STORAGE_KEYS.PENDING_INVITATION_TOKEN)
        ).toBeNull()
        expect(
          sessionStorage.getItem(LEGACY_STORAGE_KEYS.ANALYTICS_REFERRER)
        ).toBeNull()
      })

      it('does not overwrite existing new keys', () => {
        // Set both legacy and new values
        localStorage.setItem(LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID, 'legacy-team')
        localStorage.setItem(STORAGE_KEYS.CURRENT_TEAM_ID, 'new-team')

        storageUtils.migrateStorageKeys()

        // New key should retain its value, not be overwritten by legacy
        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(
          'new-team'
        )
        // Legacy key should still be removed
        expect(
          localStorage.getItem(LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID)
        ).toBeNull()
      })

      it('is idempotent - can be called multiple times safely', () => {
        localStorage.setItem(LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID, 'legacy-team')

        storageUtils.migrateStorageKeys()
        storageUtils.migrateStorageKeys()
        storageUtils.migrateStorageKeys()

        expect(localStorage.getItem(STORAGE_KEYS.CURRENT_TEAM_ID)).toBe(
          'legacy-team'
        )
      })

      it('handles migration errors gracefully', () => {
        localStorage.setItem(LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID, 'team')

        // Mock Storage.prototype.setItem to fail for the new team-id key
        const originalSetItem = Storage.prototype.setItem
        jest.spyOn(Storage.prototype, 'setItem').mockImplementation(function (
          this: Storage,
          key: string,
          value: string
        ) {
          if (key === STORAGE_KEYS.CURRENT_TEAM_ID) {
            throw new Error('Storage error')
          }
          originalSetItem.call(this, key, value)
        })

        // Should not throw
        expect(() => storageUtils.migrateStorageKeys()).not.toThrow()
      })
    })
  })
})

describe('STORAGE_KEYS', () => {
  it('all keys have vx_ prefix', () => {
    Object.values(STORAGE_KEYS).forEach(key => {
      expect(key).toMatch(/^vx_/)
    })
  })

  it('has expected keys defined', () => {
    expect(STORAGE_KEYS.CURRENT_TEAM_ID).toBeDefined()
    expect(STORAGE_KEYS.PENDING_INVITATION_TOKEN).toBeDefined()
    expect(STORAGE_KEYS.INVITATION_BANNER_DISMISSED).toBeDefined()
    expect(STORAGE_KEYS.ANALYTICS_REFERRER).toBeDefined()
    expect(STORAGE_KEYS.PURCHASE_TRACKED).toBeDefined()
    expect(STORAGE_KEYS.COOKIE_CONSENT).toBeDefined()
  })
})
