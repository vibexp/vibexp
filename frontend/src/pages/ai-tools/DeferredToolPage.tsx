import { ArrowLeft } from 'lucide-react'
import { Link } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

interface DeferredToolPageProps {
  title: string
  description: string
  backHref: string
}

/**
 * Shared placeholder for v2 AI Tools subpages that are intentionally
 * deferred from the rewrite (Slices 11 & 12 — sessions + setup deferred).
 * Phase 4 removed the v1 escape hatch since v1 routes now redirect to v2.
 */
export function DeferredToolPage({
  title,
  description,
  backHref,
}: Readonly<DeferredToolPageProps>) {
  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        asChild
        className="text-muted-foreground"
      >
        <Link to={backHref}>
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Link>
      </Button>

      <PageHeader title={title} description={description} />

      <Alert>
        <AlertTitle>Coming soon</AlertTitle>
        <AlertDescription>
          <p>
            This page is part of the AI Tools module that hasn&apos;t been
            rebuilt yet. Tracked on the v2 rewrite epic as deferred from slices
            11/12.
          </p>
        </AlertDescription>
      </Alert>
    </div>
  )
}
