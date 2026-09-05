import { useCallback, useEffect, useState } from 'react'

import type { StorageKey } from '../constants/storageKeys'
import { storage } from '../utils/storage'

/**
 * React hook for using localStorage with React state
 *
 * Values are JSON-serialized on write and parsed on read (via the shared
 * `storage` client), so booleans, numbers and objects round-trip intact.
 *
 * @param key - The localStorage key to use (prefer constants from STORAGE_KEYS)
 * @param initialValue - The initial value if no value is stored
 * @returns A tuple of [storedValue, setValue] similar to useState
 *
 * @example
 * ```tsx
 * import { useLocalStorage } from '@/hooks/useLocalStorage'
 * import { STORAGE_KEYS } from '@/constants/storageKeys'
 *
 * const [collapsed, setCollapsed] = useLocalStorage(STORAGE_KEYS.NAV_COLLAPSED, false)
 * ```
 */
export function useLocalStorage<T>(key: StorageKey, initialValue: T) {
  const [storedValue, setStoredValue] = useState<T>(() => {
    try {
      return storage.getJSON<T>(key) ?? initialValue
    } catch (error) {
      console.error(`Error reading storage key "${key}":`, error)
      return initialValue
    }
  })

  // Stable identity (like useState's setter) so consumers can list it in
  // effect/memo dependencies without re-running on every render. Functional
  // updates resolve against the latest state, not a stale closure.
  const setValue = useCallback(
    (value: T | ((val: T) => T)) => {
      setStoredValue(prev => {
        const next =
          typeof value === 'function' ? (value as (val: T) => T)(prev) : value
        try {
          storage.set(key, next)
        } catch (error) {
          console.error(`Error setting storage key "${key}":`, error)
        }
        return next
      })
    },
    [key]
  )

  useEffect(() => {
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === key && e.newValue !== null) {
        try {
          setStoredValue(JSON.parse(e.newValue) as T)
        } catch (error) {
          console.error(`Error parsing storage key "${key}":`, error)
        }
      }
    }

    window.addEventListener('storage', handleStorageChange)
    return () => {
      window.removeEventListener('storage', handleStorageChange)
    }
  }, [key])

  return [storedValue, setValue] as const
}
