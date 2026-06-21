import { useEffect, useState } from 'react'

import type { StorageKey } from '../constants/storageKeys'
import { storage } from '../utils/storage'

/**
 * React hook for using localStorage with React state
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
 * const [theme, setTheme] = useLocalStorage(STORAGE_KEYS.THEME, 'light')
 * ```
 */
export function useLocalStorage<T>(key: StorageKey, initialValue: T) {
  const [storedValue, setStoredValue] = useState<T>(() => {
    try {
      const item = storage.get(key)

      return (item as T | null) ?? initialValue
    } catch (error) {
      console.error(`Error reading storage key "${key}":`, error)
      return initialValue
    }
  })

  const setValue = (value: T | ((val: T) => T)) => {
    try {
      const valueToStore =
        value instanceof Function ? value(storedValue) : value
      setStoredValue(valueToStore)
      storage.set(key, valueToStore)
    } catch (error) {
      console.error(`Error setting storage key "${key}":`, error)
    }
  }

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
