import { ArrowLeft } from 'lucide-react'
import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { VersionHistoryPage } from '@/features/version-history'
import { createArtifactVersionSource } from '@/services/versionService'

// Thin artifact wrapper around the resource-agnostic VersionHistoryPage. The
// `/artifacts/:project/:slug/versions` route stays for artifacts; adding e.g.
// `/prompts/:…/versions` later is just another wrapper + version source.
export function ArtifactVersions() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()

  const decodedProject = project ? decodeURIComponent(project) : ''
  const decodedSlug = slug ? decodeURIComponent(slug) : ''
  const base = `/artifacts/${encodeURIComponent(decodedProject)}/${encodeURIComponent(decodedSlug)}`

  const source = useMemo(
    () =>
      createArtifactVersionSource({
        teamId: currentTeam?.id ?? '',
        projectId: decodedProject,
        slug: decodedSlug,
        backHref: base,
      }),
    [currentTeam?.id, decodedProject, decodedSlug, base]
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

  if (!project || !slug || !currentTeam) {
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
        <Button variant="outline" onClick={() => void navigate('/artifacts')}>
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
    )
  }

  return <VersionHistoryPage source={source} />
}
