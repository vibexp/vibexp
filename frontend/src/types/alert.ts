// Alert System types
export type AlertType = 'success' | 'error' | 'warning' | 'info'

export interface AlertOptions {
  title?: string
  message: string
  type: AlertType
  duration?: number // ms, 0 = no auto-dismiss
  persistent?: boolean // prevents auto-dismiss
}

export interface Alert {
  id: string
  title?: string
  message: string
  type: AlertType
  duration: number
  persistent: boolean
  createdAt: number
}

export interface AlertContextValue {
  alerts: Alert[]
  showAlert: (options: AlertOptions) => string
  dismissAlert: (id: string) => void
  clearAll: () => void
}

export interface UseAlerts {
  showAlert: (options: AlertOptions) => string
  showSuccess: (message: string, title?: string) => string
  showError: (message: string, title?: string) => string
  showWarning: (message: string, title?: string) => string
  showInfo: (message: string, title?: string) => string
  dismissAlert: (id: string) => void
  clearAll: () => void
  alerts: Alert[]
}
