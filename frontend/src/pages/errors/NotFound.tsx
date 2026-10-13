import { ArrowLeft } from 'lucide-react'
import { Link } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'

/**
 * The 404 page: one message and one way out. It says "not found" once — in the
 * page heading — with a line of explanation under it, not a second time inside
 * a boxed empty state.
 */
export function NotFound() {
  return (
    <div>
      <PageHeader
        title="Page not found"
        description="This page doesn’t exist. It may have been moved or deleted, or the address may be mistyped."
      />
      <Button variant="outline" asChild>
        <Link to="/">
          <ArrowLeft className="mr-2 size-4" />
          Back to dashboard
        </Link>
      </Button>
    </div>
  )
}
