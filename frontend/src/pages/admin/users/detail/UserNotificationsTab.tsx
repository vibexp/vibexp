import { Lock } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime } from '@/lib/time'
import { NOTIFICATION_TYPE_LABELS } from '@/pages/settings/notifications/notificationTypeLabels'
import type { AdminUserNotificationPreferences } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const EMAIL_CATEGORIES: {
  key: keyof AdminUserNotificationPreferences['email_notification']
  label: string
}[] = [
  { key: 'platform_announcement', label: 'Platform announcements' },
  { key: 'account_security', label: 'Account security' },
  { key: 'new_feature', label: 'New features' },
  { key: 'marketing_promotional', label: 'Marketing and promotions' },
]

const EMAIL_DELIVERY_LABELS: Record<string, string> = {
  instant: 'Instant',
  digest: 'Digest',
  none: 'None',
}

/** Whether these are untouched defaults, or when the user last changed them. */
function provenance(prefs: AdminUserNotificationPreferences): string {
  if (prefs.is_default) {
    return 'They have never changed them, so these are the defaults.'
  }
  if (prefs.updated_at)
    return `Last changed ${formatDateTime(prefs.updated_at)}.`
  return ''
}

function OnOff({ on }: Readonly<{ on: boolean }>) {
  return (
    <Badge variant={on ? 'secondary' : 'outline'} className="font-normal">
      {on ? 'On' : 'Off'}
    </Badge>
  )
}

function Row({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <div className="flex items-center justify-between gap-4 py-1">
      <span className="text-sm">{label}</span>
      <div className="flex items-center gap-2 text-sm">{children}</div>
    </div>
  )
}

/**
 * The user's notification settings as plain text and badges.
 *
 * Read-only by design: an admin can see what a user will be sent but never
 * change it, so this renders no switch, checkbox, radio or tab.
 */
export function UserNotificationsTab({ userId }: Readonly<{ userId: string }>) {
  const [prefs, setPrefs] = useState<AdminUserNotificationPreferences | null>(
    null
  )
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getUserNotificationPreferences(userId)
      .then(result => {
        if (!cancelled) setPrefs(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(
            getErrorMessage(err, 'Failed to load notification preferences')
          )
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  if (loading) {
    return (
      <div className="space-y-4" data-testid="notifications-loading">
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    )
  }

  if (error || !prefs) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load notification preferences</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  const types = Object.entries(prefs.notifications.types).sort(([a], [b]) =>
    a.localeCompare(b)
  )

  return (
    <div className="space-y-4">
      <Alert>
        <Lock className="size-4" aria-hidden />
        <AlertTitle>Read-only</AlertTitle>
        <AlertDescription>
          Read-only view of this user&apos;s notification settings.{' '}
          {provenance(prefs)}
        </AlertDescription>
      </Alert>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Email categories</CardTitle>
        </CardHeader>
        <CardContent className="divide-y">
          {EMAIL_CATEGORIES.map(({ key, label }) => (
            <Row key={key} label={label}>
              <OnOff on={prefs.email_notification[key]} />
            </Row>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Channels</CardTitle>
        </CardHeader>
        <CardContent className="divide-y">
          <Row label="In-app">
            <OnOff on={prefs.notifications.channels.in_app} />
          </Row>
          <Row label="Email">
            <OnOff on={prefs.notifications.channels.email} />
          </Row>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Per-type delivery</CardTitle>
        </CardHeader>
        <CardContent className="divide-y">
          {types.length === 0 ? (
            <p className="text-muted-foreground py-1 text-sm">
              No per-type settings.
            </p>
          ) : (
            types.map(([type, pref]) => (
              <Row key={type} label={NOTIFICATION_TYPE_LABELS[type] ?? type}>
                <span className="text-muted-foreground text-xs">In-app</span>
                <OnOff on={pref.in_app} />
                <span className="text-muted-foreground text-xs">Email</span>
                <span>{EMAIL_DELIVERY_LABELS[pref.email] ?? pref.email}</span>
              </Row>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
