import { Mail } from 'lucide-react'
import { Link } from 'react-router'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { providerTypeMeta } from '@/features/email-provider/emailProviderForm'
import { formatDateTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import {
  INSTANCE_EMAIL_SETTINGS_PATH,
  type InstanceEmailStatus,
  useInstanceEmailStatus,
} from '@/pages/admin/useInstanceEmailStatus'

/** Long provider errors are cut here; the settings page shows them in full. */
const MAX_ERROR_LENGTH = 160

function truncate(text: string): string {
  return text.length > MAX_ERROR_LENGTH
    ? `${text.slice(0, MAX_ERROR_LENGTH - 1)}…`
    : text
}

/**
 * The dashboard's "Instance email" system-health entry (#1192). It reads the
 * email status itself rather than through the dashboard overview, so an
 * overview failure never hides a mail problem. Every state links to the
 * settings page. The status is always spelled out, never colour alone.
 */
export function InstanceEmailCard() {
  const status = useInstanceEmailStatus()

  return (
    <Card data-testid="instance-email-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-xs font-medium">
          <Mail className="size-4" aria-hidden />
          Instance email
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        <StatusBody status={status} />
        <Link
          to={INSTANCE_EMAIL_SETTINGS_PATH}
          className="text-foreground inline-block text-xs underline underline-offset-4"
        >
          Email settings
        </Link>
      </CardContent>
    </Card>
  )
}

function StatusLine({
  children,
  problem = false,
}: Readonly<{ children: string; problem?: boolean }>) {
  return (
    <p
      className={cn('text-2xl font-semibold', problem && 'text-destructive')}
      data-testid="instance-email-state"
    >
      {children}
    </p>
  )
}

function Detail({ children }: Readonly<{ children: string }>) {
  return <p className="text-muted-foreground text-xs break-words">{children}</p>
}

function StatusBody({ status }: Readonly<{ status: InstanceEmailStatus }>) {
  const { state, settings } = status
  switch (state) {
    case 'loading':
      return <Skeleton className="h-12 w-full" />
    case 'unknown':
      return (
        <>
          <StatusLine>Status unavailable</StatusLine>
          <Detail>The instance email status could not be loaded.</Detail>
        </>
      )
    case 'unconfigured':
      return (
        <>
          <StatusLine problem>Not configured</StatusLine>
          <Detail>
            Instance mail is being discarded until a provider is configured.
          </Detail>
        </>
      )
    case 'failing':
      return (
        <>
          <StatusLine problem>Last send failed</StatusLine>
          {settings?.last_error_at && (
            <Detail>{`Failed at ${formatDateTime(settings.last_error_at)}`}</Detail>
          )}
          {settings?.last_error && (
            <Detail>{truncate(settings.last_error)}</Detail>
          )}
        </>
      )
    case 'healthy': {
      const provider = providerTypeMeta(settings?.provider_type ?? 'smtp').label
      return (
        <>
          <StatusLine>Configured</StatusLine>
          <Detail>
            {settings?.last_success_at
              ? `${provider} · last sent ${formatDateTime(settings.last_success_at)}`
              : `${provider} · No sends yet`}
          </Detail>
        </>
      )
    }
  }
}
