import { AlertTriangle, CheckCircle, Info, X, XCircle } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import type { Alert as AlertType } from '@/types/alert'

interface AlertProps {
  alert: AlertType
  onDismiss: (id: string) => void
}

const alertIcons = {
  success: CheckCircle,
  error: XCircle,
  warning: AlertTriangle,
  info: Info,
}

const alertStyles = {
  success: {
    container: 'bg-success-subtle border-success/30 text-foreground',
    icon: 'text-success',
    progress: 'bg-success',
  },
  error: {
    container: 'bg-destructive/10 border-destructive/30 text-foreground',
    icon: 'text-destructive',
    progress: 'bg-destructive',
  },
  warning: {
    container: 'bg-warning-subtle border-warning/30 text-foreground',
    icon: 'text-warning',
    progress: 'bg-warning',
  },
  info: {
    container: 'bg-info-subtle border-info/30 text-foreground',
    icon: 'text-info',
    progress: 'bg-info',
  },
}

export function Alert({ alert, onDismiss }: AlertProps) {
  const [isVisible, setIsVisible] = useState(false)
  const [isPaused, setIsPaused] = useState(false)
  const [progress, setProgress] = useState(100)

  const Icon = alertIcons[alert.type]
  const styles = alertStyles[alert.type]

  const handleDismiss = useCallback(() => {
    setIsVisible(false)
    setTimeout(() => {
      onDismiss(alert.id)
    }, 300) // Wait for exit animation
  }, [onDismiss, alert.id])

  // Animation for showing alert
  useEffect(() => {
    const timer = setTimeout(() => {
      setIsVisible(true)
    }, 10)
    return () => {
      clearTimeout(timer)
    }
  }, [])

  // Progress bar animation for auto-dismiss
  useEffect(() => {
    if (alert.persistent || alert.duration <= 0 || isPaused) {
      return
    }

    const startTime = Date.now()
    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime
      const remaining = Math.max(0, 100 - (elapsed / alert.duration) * 100)
      setProgress(remaining)

      if (remaining <= 0) {
        setIsVisible(false)
        setTimeout(() => {
          onDismiss(alert.id)
        }, 300)
      }
    }, 50)

    return () => {
      clearInterval(interval)
    }
  }, [alert.persistent, alert.duration, isPaused, onDismiss, alert.id])

  const handleMouseEnter = () => {
    if (!alert.persistent && alert.duration > 0) {
      setIsPaused(true)
    }
  }

  const handleMouseLeave = () => {
    if (!alert.persistent && alert.duration > 0) {
      setIsPaused(false)
    }
  }

  return (
    <div
      className={`transform transition-all duration-300 ease-in-out ${
        isVisible
          ? 'translate-x-0 opacity-100 scale-100'
          : 'translate-x-full opacity-0 scale-95'
      }`}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      role="alert"
      aria-live="polite"
      aria-atomic="true"
    >
      <div
        className={`relative max-w-sm w-full border rounded-lg shadow-lg overflow-hidden ${styles.container}`}
      >
        {/* Progress bar for auto-dismiss */}
        {!alert.persistent && alert.duration > 0 && (
          <div className="absolute top-0 left-0 h-1 bg-muted">
            <div
              className={`h-full transition-all duration-75 ease-linear ${styles.progress}`}
              style={{ width: `${String(progress)}%` }}
            />
          </div>
        )}

        <div className="p-4">
          <div className="flex items-start">
            <div className="flex-shrink-0">
              <Icon className={`h-5 w-5 ${styles.icon}`} aria-hidden="true" />
            </div>
            <div className="ml-3 flex-1">
              {alert.title && (
                <h3 className="text-sm font-medium mb-1">{alert.title}</h3>
              )}
              <p className="text-sm">{alert.message}</p>
            </div>
            <div className="ml-4 flex-shrink-0">
              <button
                type="button"
                className="inline-flex rounded-md p-1.5 hover:bg-black/5 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-ring transition-colors"
                onClick={handleDismiss}
                aria-label="Dismiss alert"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
