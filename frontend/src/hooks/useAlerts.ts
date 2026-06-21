import { useCallback } from 'react'

import { useAlertContext } from '../contexts/AlertContext'
import type { UseAlerts } from '../types'

export function useAlerts(): UseAlerts {
  const { alerts, showAlert, dismissAlert, clearAll } = useAlertContext()

  // Convenience method for success alerts
  const showSuccess = useCallback(
    (message: string, title?: string): string => {
      return showAlert({
        message,
        title,
        type: 'success',
      })
    },
    [showAlert]
  )

  // Convenience method for error alerts
  const showError = useCallback(
    (message: string, title?: string): string => {
      return showAlert({
        message,
        title,
        type: 'error',
      })
    },
    [showAlert]
  )

  // Convenience method for warning alerts
  const showWarning = useCallback(
    (message: string, title?: string): string => {
      return showAlert({
        message,
        title,
        type: 'warning',
      })
    },
    [showAlert]
  )

  // Convenience method for info alerts
  const showInfo = useCallback(
    (message: string, title?: string): string => {
      return showAlert({
        message,
        title,
        type: 'info',
      })
    },
    [showAlert]
  )

  return {
    alerts,
    showAlert,
    showSuccess,
    showError,
    showWarning,
    showInfo,
    dismissAlert,
    clearAll,
  }
}
