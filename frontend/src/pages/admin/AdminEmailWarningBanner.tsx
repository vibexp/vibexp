import { AlertTriangle, X } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { formatDateTime } from '@/lib/time'
import {
  INSTANCE_EMAIL_SETTINGS_PATH,
  useInstanceEmailStatus,
} from '@/pages/admin/useInstanceEmailStatus'
import { sessionStore } from '@/utils/storage'

function readDismissed(): boolean {
  return sessionStore.get(STORAGE_KEYS.ADMIN_EMAIL_BANNER_DISMISSED) === 'true'
}

/**
 * Admin-shell warning (#1192) shown while instance email is not configured or
 * its last send failed — otherwise the stub provider discards the instance's
 * mail and nothing in the UI says so.
 *
 * Hidden when the status is healthy or could not be read (a failed read is not
 * evidence of a problem), on the settings page itself (it shows the full
 * status), and for the rest of the browser session once dismissed.
 */
export function AdminEmailWarningBanner() {
  const { pathname } = useLocation()
  const { state, settings } = useInstanceEmailStatus()
  const [dismissed, setDismissed] = useState(readDismissed)

  if (dismissed) return null
  if (pathname.replace(/\/+$/, '') === INSTANCE_EMAIL_SETTINGS_PATH) return null
  if (state !== 'unconfigured' && state !== 'failing') return null

  const dismiss = () => {
    sessionStore.set(STORAGE_KEYS.ADMIN_EMAIL_BANNER_DISMISSED, true)
    setDismissed(true)
  }

  const failedAt = settings?.last_error_at
  const failedAtText = failedAt
    ? `It failed at ${formatDateTime(failedAt)}. `
    : ''
  return (
    <Alert
      variant="destructive"
      className="mb-6 pr-12"
      data-testid="admin-email-warning"
    >
      {/* Before the icon, so the Alert's `[&>svg~*]` indent skips it. */}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute top-2 right-2 size-8"
        aria-label="Dismiss"
        onClick={dismiss}
      >
        <X className="size-4" aria-hidden />
      </Button>
      <AlertTriangle className="size-4" aria-hidden />
      <AlertTitle>
        {state === 'unconfigured'
          ? 'Instance email is not configured'
          : 'The last instance email send failed'}
      </AlertTitle>
      <AlertDescription className="space-y-1">
        <p>
          {state === 'unconfigured'
            ? 'Invitations, notifications and digests from teams without their own mail provider are being discarded.'
            : `${failedAtText}Invitations, notifications and digests from teams without their own mail provider may not be delivered.`}
        </p>
        <Link
          to={INSTANCE_EMAIL_SETTINGS_PATH}
          className="font-medium underline underline-offset-4"
        >
          Configure email
        </Link>
      </AlertDescription>
    </Alert>
  )
}
