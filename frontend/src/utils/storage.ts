/**
 * Type-Safe Storage Utilities
 *
 * Provides type-safe wrappers around localStorage and sessionStorage with:
 * - Type-safe key access using STORAGE_KEYS constants
 * - Automatic JSON serialization/deserialization
 * - Error handling for quota exceeded, security errors, etc.
 * - Fallback mechanisms for when storage is unavailable
 *
 * Usage:
 * ```ts
 * import { storage, sessionStore } from '@/utils/storage'
 * import { STORAGE_KEYS } from '@/constants/storageKeys'
 *
 * // localStorage
 * storage.set(STORAGE_KEYS.CURRENT_TEAM_ID, 'team-123')
 * const teamId = storage.get(STORAGE_KEYS.CURRENT_TEAM_ID) // string | null
 * storage.remove(STORAGE_KEYS.CURRENT_TEAM_ID)
 *
 * // sessionStorage
 * sessionStore.set(STORAGE_KEYS.ANALYTICS_REFERRER, '/previous-page')
 * const referrer = sessionStore.get(STORAGE_KEYS.ANALYTICS_REFERRER)
 * ```
 */

import {
  LEGACY_STORAGE_KEYS,
  STORAGE_KEYS,
  type StorageKey,
} from '../constants/storageKeys'

// Re-export STORAGE_KEYS for convenience
export { STORAGE_KEYS }

/**
 * Base storage interface with type-safe methods
 */
interface StorageClient {
  /**
   * Get a raw string value from storage
   * @param key - The storage key to retrieve
   * @returns The raw string value or null if not found
   */
  get(key: StorageKey): string | null

  /**
   * Get a JSON-parsed value from storage with type safety
   * @param key - The storage key to retrieve
   * @returns The parsed value or null if not found/invalid
   */
  getJSON<T>(key: StorageKey): T | null

  /**
   * Set a value in storage
   * @param key - The storage key to set
   * @param value - The value to store (will be JSON serialized if not a string)
   */
  set(key: StorageKey, value: unknown): void

  /**
   * Remove a value from storage
   * @param key - The storage key to remove
   */
  remove(key: StorageKey): void

  /**
   * Clear all values from storage
   * Use with caution - this removes ALL items
   */
  clear(): void

  /**
   * Check if a key exists in storage
   * @param key - The storage key to check
   */
  has(key: StorageKey): boolean
}

/**
 * Create a typed storage wrapper around a Storage implementation
 */
function createStorageClient(baseStorage: Storage): StorageClient {
  /**
   * Get storage type name for error messages
   */
  function getStorageTypeName(): string {
    return baseStorage === localStorage ? 'local' : 'session'
  }

  /**
   * Safely get a raw string value from storage
   */
  function get(key: StorageKey): string | null {
    try {
      return baseStorage.getItem(key)
    } catch (error) {
      console.error(
        `Failed to read from ${getStorageTypeName()} storage for key "${key}":`,
        error
      )
      return null
    }
  }

  /**
   * Safely get a JSON-parsed value from storage
   * Returns null if the key doesn't exist, value is null, or parsing fails
   */
  function getJSON<T>(key: StorageKey): T | null {
    try {
      const item = baseStorage.getItem(key)
      if (item === null) {
        return null
      }

      return JSON.parse(item) as T
    } catch (error) {
      // Handle storage access errors or JSON parsing errors
      console.error(
        `Failed to read/parse from ${getStorageTypeName()} storage for key "${key}":`,
        error
      )
      return null
    }
  }

  /**
   * Safely set a value in storage with JSON serialization
   */
  function set(key: StorageKey, value: unknown): void {
    try {
      const serialized =
        typeof value === 'string' ? value : JSON.stringify(value)
      baseStorage.setItem(key, serialized)
    } catch (error) {
      // Handle storage errors (quota exceeded, security restrictions, etc.)
      if (error instanceof Error && error.name === 'QuotaExceededError') {
        console.error(
          `Storage quota exceeded for key "${key}". Consider clearing old data.`
        )
      } else {
        console.error(
          `Failed to write to ${getStorageTypeName()} storage for key "${key}":`,
          error
        )
      }
    }
  }

  /**
   * Safely remove a value from storage
   */
  function remove(key: StorageKey): void {
    try {
      baseStorage.removeItem(key)
    } catch (error) {
      console.error(
        `Failed to remove from ${getStorageTypeName()} storage for key "${key}":`,
        error
      )
    }
  }

  /**
   * Safely clear all items from storage
   */
  function clear(): void {
    try {
      baseStorage.clear()
    } catch (error) {
      console.error(`Failed to clear ${getStorageTypeName()} storage:`, error)
    }
  }

  /**
   * Check if a key exists in storage
   */
  function has(key: StorageKey): boolean {
    try {
      return baseStorage.getItem(key) !== null
    } catch (error) {
      console.error(
        `Failed to check ${getStorageTypeName()} storage for key "${key}":`,
        error
      )
      return false
    }
  }

  return { get, getJSON, set, remove, clear, has }
}

/**
 * Type-safe localStorage wrapper
 * Use for persistent data that should survive browser restarts
 */
export const storage = createStorageClient(localStorage)

/**
 * Type-safe sessionStorage wrapper
 * Use for temporary data that should be cleared when the browser tab closes
 */
export const sessionStore = createStorageClient(sessionStorage)

/**
 * Utility functions for common storage operations
 */
export const storageUtils = {
  /**
   * Clear all VibeXP-related storage keys (preserves third-party keys)
   * Useful for logout operations
   */
  clearVibeXPData(): void {
    Object.values(STORAGE_KEYS).forEach(key => {
      if (storage.has(key)) {
        storage.remove(key)
      }
      if (sessionStore.has(key)) {
        sessionStore.remove(key)
      }
    })
  },

  /**
   * Get all VibeXP storage keys and their values
   * Useful for debugging and auditing
   *
   * WARNING: This may expose sensitive data including authentication tokens.
   * Only use in secure contexts (dev tools, secure admin panels).
   */
  getAllVibeXPData(): Record<
    string,
    { local: string | null; session: string | null }
  > {
    // Warn about sensitive data exposure in development
    if (process.env.NODE_ENV === 'development') {
      console.warn(
        'getAllVibeXPData() may expose sensitive data including authentication tokens. Only use for debugging.'
      )
    }

    const result: Record<
      string,
      { local: string | null; session: string | null }
    > = {}

    Object.values(STORAGE_KEYS).forEach(key => {
      result[key] = {
        local: storage.get(key),
        session: sessionStore.get(key),
      }
    })

    return result
  },

  /**
   * Check if storage is available (not in privacy mode, etc.)
   */
  isStorageAvailable(): { local: boolean; session: boolean } {
    const testKey = '__storage_test__'
    let localAvailable = false
    let sessionAvailable = false

    // Test localStorage
    try {
      localStorage.setItem(testKey, '1')
      localStorage.removeItem(testKey)
      localAvailable = true
    } catch {
      localAvailable = false
    }

    // Test sessionStorage
    try {
      sessionStorage.setItem(testKey, '1')
      sessionStorage.removeItem(testKey)
      sessionAvailable = true
    } catch {
      sessionAvailable = false
    }

    return { local: localAvailable, session: sessionAvailable }
  },

  /**
   * Migrate data from legacy storage keys to new prefixed keys.
   * This should be called once on app initialization to ensure
   * existing user data is preserved when upgrading to the new key format.
   *
   * Migration is idempotent - if new keys already exist, legacy keys are just cleaned up.
   */
  migrateStorageKeys(): void {
    const migrations: {
      legacy: string
      current: StorageKey
      storage: Storage
    }[] = [
      // localStorage migrations
      {
        legacy: LEGACY_STORAGE_KEYS.CURRENT_TEAM_ID,
        current: STORAGE_KEYS.CURRENT_TEAM_ID,
        storage: localStorage,
      },
      {
        legacy: LEGACY_STORAGE_KEYS.COOKIE_CONSENT,
        current: STORAGE_KEYS.COOKIE_CONSENT,
        storage: localStorage,
      },
      // sessionStorage migrations
      {
        legacy: LEGACY_STORAGE_KEYS.PENDING_INVITATION_TOKEN,
        current: STORAGE_KEYS.PENDING_INVITATION_TOKEN,
        storage: sessionStorage,
      },
      {
        legacy: LEGACY_STORAGE_KEYS.INVITATION_BANNER_DISMISSED,
        current: STORAGE_KEYS.INVITATION_BANNER_DISMISSED,
        storage: sessionStorage,
      },
      {
        legacy: LEGACY_STORAGE_KEYS.ANALYTICS_REFERRER,
        current: STORAGE_KEYS.ANALYTICS_REFERRER,
        storage: sessionStorage,
      },
      {
        legacy: LEGACY_STORAGE_KEYS.PURCHASE_TRACKED,
        current: STORAGE_KEYS.PURCHASE_TRACKED,
        storage: sessionStorage,
      },
    ]

    // INVITATION_BANNER_DISMISSED's stored shape changed from a Unix-timestamp
    // string to a JSON string[] of dismissed invitation IDs (#1416). Legacy
    // timestamp values would otherwise survive migration as semantically
    // incompatible data, so we only migrate values that already parse as a
    // JSON array.
    const isJsonStringArray = (raw: string): boolean => {
      try {
        const parsed: unknown = JSON.parse(raw)
        return (
          Array.isArray(parsed) &&
          parsed.every(item => typeof item === 'string')
        )
      } catch {
        return false
      }
    }

    for (const { legacy, current, storage: store } of migrations) {
      try {
        const legacyValue = store.getItem(legacy)
        if (legacyValue !== null) {
          const skipMigration =
            current === STORAGE_KEYS.INVITATION_BANNER_DISMISSED &&
            !isJsonStringArray(legacyValue)

          // Only migrate if the new key doesn't already have a value AND the
          // legacy value's shape still matches the current schema.
          if (!skipMigration && store.getItem(current) === null) {
            store.setItem(current, legacyValue)
          }
          // Always remove the legacy key to prevent confusion
          store.removeItem(legacy)
        }
      } catch {
        // Ignore migration errors - storage may be unavailable
      }
    }

    // Auth tokens are never stored client-side: the app authenticates via an
    // httpOnly session cookie. Both the legacy `auth_token` and the old
    // `vx_auth_token` keys are pure cleanup — delete (never re-create) them so
    // any returning user from the pre-cookie auth flow gets the stale token
    // wiped. The literal 'vx_auth_token' is no longer in STORAGE_KEYS.
    try {
      localStorage.removeItem(LEGACY_STORAGE_KEYS.AUTH_TOKEN)
      localStorage.removeItem('vx_auth_token')
    } catch {
      // Ignore storage errors - storage may be unavailable
    }
  },
}
