import type { ReactNode } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

/**
 * The loading / error / empty frame every admin team configuration tab shares
 * (#1142). A tab's failure renders inside the tab only, so the page header and
 * the other tabs are unaffected.
 */
export function AdminConfigPanel({
  loading,
  error,
  errorTitle,
  empty = false,
  emptyMessage,
  children,
}: Readonly<{
  loading: boolean
  error: string | null
  errorTitle: string
  empty?: boolean
  emptyMessage?: string
  children: ReactNode
}>) {
  if (loading) {
    return (
      <div className="space-y-3" data-testid="config-loading">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-32 w-full" />
      </div>
    )
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{errorTitle}</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }
  if (empty) {
    return (
      <Card>
        <CardContent
          className="text-muted-foreground p-8 text-center text-sm"
          data-testid="config-empty"
        >
          {emptyMessage}
        </CardContent>
      </Card>
    )
  }
  return <div className="space-y-4">{children}</div>
}

/** A titled card section inside a configuration tab. */
export function ConfigSection({
  title,
  aside,
  children,
}: Readonly<{ title: string; aside?: ReactNode; children: ReactNode }>) {
  return (
    <Card>
      <CardContent className="space-y-4 py-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">{title}</h3>
          {aside}
        </div>
        {children}
      </CardContent>
    </Card>
  )
}
