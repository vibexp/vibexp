import { Bell, Lock, Mail } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { fcmService } from '@/services/notifications/fcm'
import { preferencesService } from '@/services/preferencesService'
import type {
  EmailNotificationPreferences,
  NotificationPreferences,
  NotificationTypePreference,
} from '@/types/preferences'

// ---------------------------------------------------------------------------
// Shared row component
// ---------------------------------------------------------------------------

interface RowProps {
  id: string
  label: string
  description: string
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
}

function PreferenceRow({
  id,
  label,
  description,
  checked,
  disabled,
  onChange,
}: RowProps) {
  return (
    <div className="flex items-start justify-between gap-4 py-3">
      <div className="flex-1 space-y-1">
        <label
          htmlFor={id}
          className="flex items-center gap-2 text-sm font-medium"
        >
          {label}
          {disabled && (
            <Lock
              className="text-muted-foreground size-3"
              aria-label="Locked"
            />
          )}
        </label>
        <p className="text-muted-foreground text-sm">{description}</p>
      </div>
      <Switch
        id={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Browser push card
// ---------------------------------------------------------------------------

const NOTIFICATION_TYPE_LABELS: Record<string, string> = {
  'feed.item.created': 'New feed items',
  'feed.reply.created': 'Replies to your feed posts',
  'team.invitation': 'Team invitations',
}

interface BrowserPushCardProps {
  notifPrefs: NotificationPreferences | null
  onChannelChange: (enabled: boolean) => void
  onTypeChange: (typeName: string, enabled: boolean) => void
  permissionDenied: boolean
}

function BrowserPushCard({
  notifPrefs,
  onChannelChange,
  onTypeChange,
  permissionDenied,
}: BrowserPushCardProps) {
  const masterEnabled = notifPrefs?.channels.web_push ?? false

  if (!fcmService.isFCMConfigured()) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Bell className="text-muted-foreground size-5" />
            <CardTitle>Browser notifications</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">
            Browser notifications require configuration. Contact your
            administrator to enable this feature.
          </p>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Bell className="text-muted-foreground size-5" />
          <CardTitle>Browser notifications</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="divide-y">
        <PreferenceRow
          id="browser_push_master"
          label="Enable browser notifications"
          description="Receive OS-level push notifications even when the app is in the background."
          checked={masterEnabled}
          onChange={onChannelChange}
        />

        {permissionDenied && (
          <Alert variant="destructive" className="mt-3">
            <AlertTitle>Permission blocked</AlertTitle>
            <AlertDescription>
              Browser blocked notifications. To enable, allow notifications for
              this site in your browser settings.
            </AlertDescription>
          </Alert>
        )}

        {masterEnabled && notifPrefs && (
          <div className="pt-3">
            <p className="text-muted-foreground mb-3 text-xs font-medium uppercase tracking-wide">
              Notify me about
            </p>
            <div className="space-y-3">
              {Object.entries(notifPrefs.types ?? {}).map(
                ([typeName, typePrefs]) => (
                  <WebPushTypeRow
                    key={typeName}
                    typeName={typeName}
                    typePrefs={typePrefs}
                    onTypeChange={onTypeChange}
                  />
                )
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

interface WebPushTypeRowProps {
  typeName: string
  typePrefs: NotificationTypePreference
  onTypeChange: (typeName: string, enabled: boolean) => void
}

function WebPushTypeRow({
  typeName,
  typePrefs,
  onTypeChange,
}: WebPushTypeRowProps) {
  const label = NOTIFICATION_TYPE_LABELS[typeName] ?? typeName
  const id = `web_push_type_${typeName.replace(/\./g, '_')}`

  return (
    <div className="flex items-center gap-3">
      <Checkbox
        id={id}
        checked={typePrefs.web_push}
        onCheckedChange={checked => {
          onTypeChange(typeName, checked === true)
        }}
      />
      <label htmlFor={id} className="cursor-pointer text-sm">
        {label}
      </label>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Activity email card
// ---------------------------------------------------------------------------

interface ActivityEmailCardProps {
  notifPrefs: NotificationPreferences | null
  onChannelToggle: (enabled: boolean) => void
  onTypeChange: (typeName: string, emailVal: string) => void
}

function ActivityEmailCard({
  notifPrefs,
  onChannelToggle,
  onTypeChange,
}: ActivityEmailCardProps) {
  const masterEnabled = notifPrefs?.channels.email ?? false

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Mail className="text-muted-foreground size-5" />
          <CardTitle>In-app activity email</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="divide-y">
        <PreferenceRow
          id="email_channel_master"
          label="Email me about activity in my teams"
          description="Receive email notifications for activity across your teams and feed."
          checked={masterEnabled}
          onChange={onChannelToggle}
        />

        {masterEnabled && notifPrefs && (
          <div className="pt-3">
            <p className="text-muted-foreground mb-3 text-xs font-medium uppercase tracking-wide">
              Delivery frequency per type
            </p>
            <div className="space-y-4">
              {Object.entries(notifPrefs.types ?? {}).map(
                ([typeName, typePrefs]) => (
                  <ActivityEmailTypeRow
                    key={typeName}
                    typeName={typeName}
                    typePrefs={typePrefs}
                    onTypeChange={onTypeChange}
                  />
                )
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

interface ActivityEmailTypeRowProps {
  typeName: string
  typePrefs: NotificationTypePreference
  onTypeChange: (typeName: string, emailVal: string) => void
}

function ActivityEmailTypeRow({
  typeName,
  typePrefs,
  onTypeChange,
}: ActivityEmailTypeRowProps) {
  const label = NOTIFICATION_TYPE_LABELS[typeName] ?? typeName

  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-sm">{label}</span>
      <Tabs
        value={typePrefs.email ?? 'digest'}
        onValueChange={val => {
          onTypeChange(typeName, val)
        }}
      >
        <TabsList className="h-8">
          <TabsTrigger value="instant" className="px-2 py-1 text-xs">
            Instant
          </TabsTrigger>
          <TabsTrigger value="digest" className="px-2 py-1 text-xs">
            Daily digest
          </TabsTrigger>
          <TabsTrigger value="off" className="px-2 py-1 text-xs">
            Off
          </TabsTrigger>
        </TabsList>
      </Tabs>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function NotificationPreferences() {
  const [prefs, setPrefs] = useState<EmailNotificationPreferences | null>(null)
  const [originalPrefs, setOriginalPrefs] =
    useState<EmailNotificationPreferences | null>(null)
  const [notifPrefs, setNotifPrefs] = useState<NotificationPreferences | null>(
    null
  )
  const [originalNotifPrefs, setOriginalNotifPrefs] =
    useState<NotificationPreferences | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const [permissionDenied, setPermissionDenied] = useState(false)

  const hasEmailChanges =
    prefs !== null &&
    originalPrefs !== null &&
    JSON.stringify(prefs) !== JSON.stringify(originalPrefs)

  const hasNotifChanges =
    notifPrefs !== null &&
    originalNotifPrefs !== null &&
    JSON.stringify(notifPrefs) !== JSON.stringify(originalNotifPrefs)

  const hasChanges = hasEmailChanges || hasNotifChanges

  const loadPreferences = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await preferencesService.getPreferences()
      const emailPrefs = response.preferences.email_notification
      setPrefs(emailPrefs)
      setOriginalPrefs(emailPrefs)
      const nPrefs = response.preferences.notifications ?? null
      setNotifPrefs(nPrefs)
      setOriginalNotifPrefs(nPrefs)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load preferences'
      )
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadPreferences()
  }, [loadPreferences])

  // NOTE: reads permission only on mount — if the user revokes in another tab
  // the UI won't update until remount or a toggle attempt. For live-tracking,
  // subscribe to the `permissionchange` event on the Notification object.
  useEffect(() => {
    if (typeof Notification !== 'undefined') {
      setPermissionDenied(Notification.permission === 'denied')
    }
  }, [])

  const toggleEmail = (key: keyof EmailNotificationPreferences) => {
    if (!prefs) return
    setPrefs({ ...prefs, [key]: !prefs[key] })
    setSuccessMessage(null)
  }

  const handleBrowserPushToggle = async (enabled: boolean) => {
    // Fast-path: skip the flicker of briefly clearing permissionDenied when
    // the user tries to re-enable while permission is already denied.
    if (enabled && permissionDenied) return
    setPermissionDenied(false)
    setSuccessMessage(null)

    if (enabled) {
      const granted = await fcmService.requestPermissionAndRegister()
      if (!granted) {
        setPermissionDenied(true)
        return
      }
    } else {
      await fcmService.revokeToken()
    }

    setNotifPrefs(prev => {
      if (!prev) return prev
      return {
        ...prev,
        channels: { ...prev.channels, web_push: enabled },
      }
    })
  }

  const handleWebPushTypeChange = (typeName: string, webPush: boolean) => {
    setNotifPrefs(prev => {
      if (!prev) return prev
      const updatedTypes = Object.fromEntries(
        Object.entries(prev.types ?? {}).map(([key, val]) =>
          key === typeName ? [key, { ...val, web_push: webPush }] : [key, val]
        )
      )
      return { ...prev, types: updatedTypes }
    })
    setSuccessMessage(null)
  }

  const handleEmailChannelToggle = (enabled: boolean) => {
    setNotifPrefs(prev => {
      if (!prev) return prev
      return { ...prev, channels: { ...prev.channels, email: enabled } }
    })
    setSuccessMessage(null)
  }

  const handleEmailTypeChange = (typeName: string, emailVal: string) => {
    setNotifPrefs(prev => {
      if (!prev) return prev
      const types = prev.types ?? {}
      return {
        ...prev,
        types: {
          ...types,
          [typeName]: { ...types[typeName], email: emailVal },
        },
      }
    })
    setSuccessMessage(null)
  }

  const handleSave = async () => {
    if (!prefs || !hasChanges) return
    try {
      setSaving(true)
      setError(null)
      setSuccessMessage(null)
      const response = await preferencesService.updatePreferences({
        email_notification: prefs,
        notifications: notifPrefs ?? undefined,
      })
      const updated = response.preferences.email_notification
      setPrefs(updated)
      setOriginalPrefs(updated)
      const updatedNotif = response.preferences.notifications ?? null
      setNotifPrefs(updatedNotif)
      setOriginalNotifPrefs(updatedNotif)
      setSuccessMessage('Preferences saved successfully.')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to save preferences'
      )
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (originalPrefs) {
      setPrefs(originalPrefs)
    }
    setNotifPrefs(originalNotifPrefs)
    setSuccessMessage(null)
    setPermissionDenied(false)
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Notification Preferences"
          description="Manage your email and browser notification settings."
        />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Notification Preferences"
        description="Manage your email and browser notification settings."
      />

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {successMessage && (
        <Alert>
          <AlertTitle>Saved</AlertTitle>
          <AlertDescription>{successMessage}</AlertDescription>
        </Alert>
      )}

      <Card data-testid="email-notifications-card">
        <CardHeader>
          <div className="flex items-center gap-2">
            <Mail className="text-muted-foreground size-5" />
            <CardTitle>Email notifications</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="divide-y">
          {prefs && (
            <>
              <PreferenceRow
                id="platform_announcement"
                label="Platform announcements"
                description="Updates about new platform features, maintenance schedules, and important system changes."
                checked={prefs.platform_announcement}
                onChange={() => {
                  toggleEmail('platform_announcement')
                }}
              />
              <PreferenceRow
                id="account_security"
                label="Account security"
                description="Critical security alerts including login notifications, password changes, and suspicious activity. Cannot be disabled."
                checked={prefs.account_security}
                disabled
                onChange={() => {
                  /* locked */
                }}
              />
              <PreferenceRow
                id="new_feature"
                label="New features"
                description="Be the first to know about new features and improvements."
                checked={prefs.new_feature}
                onChange={() => {
                  toggleEmail('new_feature')
                }}
              />
              <PreferenceRow
                id="marketing_promotional"
                label="Marketing & promotional"
                description="Promotional offers, tips, and resources to help you get the most out of the platform."
                checked={prefs.marketing_promotional}
                onChange={() => {
                  toggleEmail('marketing_promotional')
                }}
              />
            </>
          )}
        </CardContent>
      </Card>

      <BrowserPushCard
        notifPrefs={notifPrefs}
        onChannelChange={enabled => {
          void handleBrowserPushToggle(enabled)
        }}
        onTypeChange={handleWebPushTypeChange}
        permissionDenied={permissionDenied}
      />

      <ActivityEmailCard
        notifPrefs={notifPrefs}
        onChannelToggle={handleEmailChannelToggle}
        onTypeChange={handleEmailTypeChange}
      />

      {hasChanges && (
        <Card data-testid="save-footer">
          <CardContent className="flex items-center justify-between gap-2 pt-4">
            <p className="text-muted-foreground text-sm">
              You have unsaved changes.
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={saving}
                onClick={handleReset}
              >
                Reset
              </Button>
              <Button
                size="sm"
                disabled={saving}
                onClick={() => {
                  void handleSave()
                }}
              >
                {saving ? 'Saving…' : 'Save changes'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
