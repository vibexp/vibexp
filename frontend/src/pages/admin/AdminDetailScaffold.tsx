import { ArrowLeft } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'

/**
 * Shared shell for the admin detail pages (#316): a back link plus the
 * loading / error / content states, so `AdminUserDetail` and `AdminTeamDetail`
 * render the same way. A 404 for a non-admin or unknown id surfaces here as the
 * error alert.
 */
export function AdminDetailScaffold({
  backTo,
  backLabel,
  loading,
  error,
  errorTitle,
  children,
}: Readonly<{
  backTo: string
  backLabel: string
  loading: boolean
  error: string | null
  errorTitle: string
  children: ReactNode
}>) {
  return (
    <div className="space-y-6">
      <Link
        to={backTo}
        className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-sm"
      >
        <ArrowLeft className="size-4" />
        {backLabel}
      </Link>
      {loading && (
        <div className="space-y-3">
          <Skeleton data-testid="detail-skeleton" className="h-8 w-64" />
          <Skeleton className="h-40 w-full" />
        </div>
      )}
      {!loading && error && (
        <Alert variant="destructive">
          <AlertTitle>{errorTitle}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {!loading && !error && children}
    </div>
  )
}
