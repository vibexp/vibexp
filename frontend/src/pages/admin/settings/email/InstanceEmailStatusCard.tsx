import { AlertCircle, CheckCircle2 } from 'lucide-react'
import { Link } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { providerTypeMeta } from '@/features/email-provider/emailProviderForm'
import { formatDateTime } from '@/lib/time'
import type { AdminInstanceEmailSettings } from '@/services/adminService'

/**
 * The instance email provider's state and delivery health (#1191): not
 * configured, configured and healthy, or configured and failing.
 *
 * Adapted from the team page's `StatusCard` rather than reused: an instance
 * with nothing configured is not "inheriting" anything — its mail is discarded.
 */
export function InstanceEmailStatusCard({
  settings,
}: Readonly<{ settings: AdminInstanceEmailSettings }>) {
  if (!settings.configured) {
    return (
      <Card data-testid="instance-email-status">
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle>Instance email is not configured</CardTitle>
            <Badge variant="destructive">Not configured</Badge>
          </div>
          <CardDescription>
            Invitations, notifications and digests sent by the instance are
            discarded until a provider is configured below. Teams with their own
            email provider are unaffected.
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  // `is_healthy` is the verdict, not `last_error != null`: the last error is
  // kept after a recovery for diagnosis.
  const healthy = settings.is_healthy !== false
  const providerLabel = providerTypeMeta(settings.provider_type ?? 'smtp').label

  return (
    <Card data-testid="instance-email-status">
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>Instance provider</CardTitle>
          <Badge variant={healthy ? 'secondary' : 'destructive'}>
            {healthy ? 'Healthy' : 'Delivery failing'}
          </Badge>
        </div>
        <CardDescription>
          Sending through {providerLabel} as {settings.from_address}.{' '}
          {settings.has_credential
            ? 'A credential is stored.'
            : 'No credential is stored.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {healthy ? (
          <Alert>
            <CheckCircle2 className="size-4" />
            <AlertTitle>
              {settings.last_success_at
                ? `Last delivered successfully at ${formatDateTime(settings.last_success_at)}`
                : 'No mail sent through this provider yet'}
            </AlertTitle>
            <AlertDescription>
              {settings.last_error
                ? `An earlier send failed (${settings.last_error}), but the provider has since recovered.`
                : 'Use “Send test email” below to confirm delivery works.'}
            </AlertDescription>
          </Alert>
        ) : (
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertTitle>
              {settings.last_error_at
                ? `Last delivery failed at ${formatDateTime(settings.last_error_at)}`
                : 'Last delivery failed'}
            </AlertTitle>
            <AlertDescription>
              {settings.last_error ?? 'No error message was recorded.'}
            </AlertDescription>
          </Alert>
        )}
        <LastSaved settings={settings} />
      </CardContent>
    </Card>
  )
}

/**
 * When and by whom the configuration was last saved. A null `updated_by` is
 * the boot-time config.yaml import — or a saving admin who has since been
 * deleted; the change history below tells the two apart.
 */
function LastSaved({
  settings,
}: Readonly<{ settings: AdminInstanceEmailSettings }>) {
  if (!settings.updated_at) return null
  return (
    <p className="text-muted-foreground text-sm">
      Last saved {formatDateTime(settings.updated_at)}{' '}
      {settings.updated_by ? (
        <>
          by{' '}
          <Link
            to={`/admin/users/${settings.updated_by}`}
            className="text-foreground underline underline-offset-4"
          >
            this administrator
          </Link>
          .
        </>
      ) : (
        'by the config.yaml import (or an administrator who has since been deleted).'
      )}
    </p>
  )
}
