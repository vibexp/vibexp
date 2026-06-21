import { createPortal } from 'react-dom'

import { Alert } from '@/components/Alert'
import { useAlerts } from '@/hooks/useAlerts'

export function AlertContainer() {
  const { alerts, dismissAlert } = useAlerts()

  // Don't render anything if there are no alerts
  if (alerts.length === 0) {
    return null
  }

  // Create portal to render alerts at the top level
  return createPortal(
    <div
      className="fixed top-4 right-4 z-50 space-y-3 pointer-events-none"
      aria-live="polite"
      aria-label="Notifications"
    >
      {alerts.map(alert => (
        <div key={alert.id} className="pointer-events-auto">
          <Alert alert={alert} onDismiss={dismissAlert} />
        </div>
      ))}
    </div>,
    document.body
  )
}
