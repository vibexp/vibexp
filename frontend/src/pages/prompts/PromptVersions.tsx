import { ArrowLeft } from 'lucide-react'
import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { VersionHistoryPage } from '@/features/version-history'
import { createPromptVersionSource } from '@/services/versionService'

// Thin prompt wrapper around the resource-agnostic VersionHistoryPage, mirroring
// ArtifactVersions / BlueprintVersions / MemoryVersions. The `/prompts/:slug/versions`
// route is prompt-specific (slug-based, no project); the underlying page/diff/restore UI
// is shared with the other resources. Diffs operate on the raw body template.
export function PromptVersions() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()

  const base = `/prompts/${encodeURIComponent(slug ?? '')}`

  const source = useMemo(
    () =>
      createPromptVersionSource({
        teamId: currentTeam?.id ?? '',
        slug: slug ?? '',
        backHref: base,
      }),
    [currentTeam?.id, slug, base]
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

  if (!slug || !currentTeam) {
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
        <Button variant="outline" onClick={() => void navigate('/prompts')}>
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
    )
  }

  return <VersionHistoryPage source={source} />
}
