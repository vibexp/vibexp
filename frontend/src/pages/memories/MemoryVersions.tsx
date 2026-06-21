import { ArrowLeft } from 'lucide-react'
import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { VersionHistoryPage } from '@/features/version-history'
import { createMemoryVersionSource } from '@/services/versionService'

// Thin memory wrapper around the resource-agnostic VersionHistoryPage, mirroring
// ArtifactVersions / BlueprintVersions. The `/memories/:id/versions` route is
// memory-specific (id-based, no project/slug); the underlying page/diff/restore UI
// is shared with the other resources.
export function MemoryVersions() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()

  const base = `/memories/${encodeURIComponent(id ?? '')}`

  const source = useMemo(
    () =>
      createMemoryVersionSource({
        teamId: currentTeam?.id ?? '',
        id: id ?? '',
        backHref: base,
      }),
    [currentTeam?.id, id, base]
  )

  if (isLoadingTeam) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading version history…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (!id || !currentTeam) {
    return (
      <div className="space-y-6">
        <PageHeader title="Version history unavailable" />
        <Alert variant="destructive">
          <AlertTitle>Version history unavailable</AlertTitle>
          <AlertDescription>
            {!currentTeam
              ? 'No team available. Please select or create a team first.'
              : 'Missing required context.'}
          </AlertDescription>
        </Alert>
        <Button variant="outline" onClick={() => void navigate('/memories')}>
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
    )
  }

  return <VersionHistoryPage source={source} />
}
