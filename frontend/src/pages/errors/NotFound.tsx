import { ArrowLeft, FileQuestion } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'

export function NotFound() {
  const navigate = useNavigate()

  return (
    <div className="space-y-6">
      <PageHeader title="Page not found" />
      <EmptyState
        icon={FileQuestion}
        title="404 — page not found"
        description="The page you're looking for doesn't exist. It may have been moved, deleted, or you entered the wrong URL."
        actions={
          <Button
            variant="outline"
            onClick={() => {
              void navigate('/')
            }}
          >
            <ArrowLeft className="mr-2 size-4" />
            Back to dashboard
          </Button>
        }
      />
    </div>
  )
}
