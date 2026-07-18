import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from 'react'

import type { Alert, AlertContextValue, AlertOptions } from '../types/alert'

const AlertContext = createContext<AlertContextValue | undefined>(undefined)

// Default durations for different alert types (in milliseconds)
const DEFAULT_DURATIONS = {
  success: 5000,
  error: 8000,
  warning: 6000,
  info: 4000,
}

interface AlertProviderProps {
  children: React.ReactNode
}

export function AlertProvider({ children }: Readonly<AlertProviderProps>) {
  const [alerts, setAlerts] = useState<Alert[]>([])

  // Generate unique ID for alerts
  const generateId = useCallback(() => {
    return `alert-${String(Date.now())}-${Math.random().toString(36).slice(2, 11)}`
  }, [])

  // Remove alert by ID
  const dismissAlert = useCallback((id: string) => {
    setAlerts(prev => prev.filter(alert => alert.id !== id))
  }, [])

  // Add new alert
  const showAlert = useCallback(
    (options: AlertOptions): string => {
      const id = generateId()
      const duration = options.duration ?? DEFAULT_DURATIONS[options.type]

      const newAlert: Alert = {
        id,
        title: options.title,
        message: options.message,
        type: options.type,
        duration,
        persistent: options.persistent ?? false,
        createdAt: Date.now(),
      }

      setAlerts(prev => [...prev, newAlert])

      // Set up auto-dismiss if not persistent and has duration > 0
      if (!newAlert.persistent && duration > 0) {
        setTimeout(() => {
          dismissAlert(id)
        }, duration)
      }

      return id
    },
    [generateId, dismissAlert]
  )

  // Clear all alerts
  const clearAll = useCallback(() => {
    setAlerts([])
  }, [])

  // Cleanup old alerts (older than 30 seconds) to prevent memory leaks
  useEffect(() => {
    const interval = setInterval(() => {
      const thirtySecondsAgo = Date.now() - 30000
      setAlerts(prev =>
        prev.filter(
          alert => alert.createdAt > thirtySecondsAgo || alert.persistent
        )
      )
    }, 10000) // Check every 10 seconds

    return () => {
      clearInterval(interval)
    }
  }, [])

  const value: AlertContextValue = {
    alerts,
    showAlert,
    dismissAlert,
    clearAll,
  }

  return <AlertContext.Provider value={value}>{children}</AlertContext.Provider>
}

export function useAlertContext(): AlertContextValue {
  const context = useContext(AlertContext)
  if (context === undefined) {
    throw new Error('useAlertContext must be used within an AlertProvider')
  }
  return context
}
