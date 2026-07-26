import { AlertCircle, CheckCircle2 } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { TeamEmailProviderResponse } from '@/services/emailProviderService'

import { providerTypeMeta } from './emailProviderForm'

/**
 * The read-only halves of the email-provider page: the status card every role
 * sees, and the configuration summary shown instead of the form to members.
 *
 * Split out of `EmailProvider.tsx` to keep that file under the project's
 * `max-lines` / `max-lines-per-function` limits, which are ERRORS here.
 */

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return 'unknown time'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime())
    ? 'unknown time'
    : parsed.toLocaleString()
}

/**
 * Renders the three states the single GET response can describe: inheriting the
 * instance provider, a healthy team provider, and one whose delivery is failing.
 */
export function StatusCard({
  provider,
}: Readonly<{ provider: TeamEmailProviderResponse }>) {
  if (!provider.configured) {
    return (
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle>Using the instance default</CardTitle>
            <Badge variant="secondary">Instance provider</Badge>
          </div>
          <CardDescription>
            This team has no email provider of its own, so its mail is sent by
            the provider the instance operator configured.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">
            Mail is currently sent from{' '}
            <span className="text-foreground font-medium">
              {provider.effective_from_address}
            </span>
            . Configure a provider below to send the team&apos;s mail through
            your own instead.
          </p>
        </CardContent>
      </Card>
    )
  }

  // `is_healthy` is the verdict, NOT `last_error != null`: the backend keeps the
  // last error after a recovery so it stays readable for diagnosis, so treating
  // its presence as "currently broken" would pin a stale failure to a working
  // provider.
  const healthy = provider.is_healthy !== false

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>Team provider</CardTitle>
          <Badge variant={healthy ? 'secondary' : 'destructive'}>
            {healthy ? 'Healthy' : 'Delivery failing'}
          </Badge>
        </div>
        <CardDescription>
          Sending through{' '}
          {providerTypeMeta(provider.provider_type ?? 'smtp').label} as{' '}
          {provider.effective_from_address}.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {!healthy && provider.last_error && (
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertTitle>
              Last delivery failed at {formatTimestamp(provider.last_error_at)}
            </AlertTitle>
            <AlertDescription>
              {provider.last_error}
              <p className="mt-2">
                Team mail is not falling back to the instance provider — it
                stops until this is fixed.
              </p>
            </AlertDescription>
          </Alert>
        )}

        {healthy && (
          <Alert>
            <CheckCircle2 className="size-4" />
            <AlertTitle>
              {provider.last_success_at
                ? `Last delivered successfully at ${formatTimestamp(provider.last_success_at)}`
                : 'No mail sent through this provider yet'}
            </AlertTitle>
            <AlertDescription>
              {provider.last_error
                ? `An earlier send failed (${provider.last_error}), but the provider has since recovered.`
                : 'Use “Send test email” below to confirm delivery works.'}
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}

/**
 * What a member sees in place of the form. Gating here is convenience only —
 * every write is authorized server-side regardless.
 */
export function ReadOnlyCard({
  provider,
}: Readonly<{ provider: TeamEmailProviderResponse }>) {
  const rows: { label: string; value: string }[] = provider.configured
    ? [
        {
          label: 'Provider',
          value: providerTypeMeta(provider.provider_type ?? 'smtp').label,
        },
        { label: 'From address', value: provider.from_address ?? '—' },
        { label: 'Display name', value: provider.from_name ?? '—' },
        { label: 'Reply-To', value: provider.reply_to ?? '—' },
        {
          label: 'Credential',
          value: provider.has_credential ? 'Stored' : 'Not set',
        },
      ]
    : [{ label: 'From address', value: provider.effective_from_address }]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Configuration</CardTitle>
        <CardDescription>
          Only team owners and admins can change the email provider.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="grid gap-3 sm:grid-cols-2">
          {rows.map(row => (
            <div key={row.label} className="space-y-1">
              <dt className="text-muted-foreground text-sm">{row.label}</dt>
              <dd className="text-sm font-medium">{row.value}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}
