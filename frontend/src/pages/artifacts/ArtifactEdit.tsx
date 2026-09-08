import { AlertCircle, ArrowLeft } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { ReadingPage } from '@/components/patterns/reading-page'
import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  formHeading,
  getResourceDescriptor,
  ResourceFormReadingPage,
} from '@/components/patterns/resource'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toArtifactRequest } from '@/pages/artifacts/artifactRequest'
import type { Artifact } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const descriptor = getResourceDescriptor('artifact')

export function ArtifactEdit() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [artifact, setArtifact] = useState<Artifact | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)

  const loadAll = useCallback(async () => {
    if (isLoadingTeam) return
    if (!project || !slug) {
      setError('Missing required context')
      setLoading(false)
      return
    }
    if (!currentTeam) {
      setError('No team available. Please select or create a team first.')
      setLoading(false)
      return
    }
    try {
      setLoading(true)
      setError(null)
      const a = await artifactService.getArtifact(
        currentTeam.id,
        decodeURIComponent(project),
        decodeURIComponent(slug)
      )
      setArtifact(a)
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to load artifact'))
      handleError(err, 'Failed to load artifact')
    } finally {
      setLoading(false)
    }
  }, [project, slug, currentTeam, isLoadingTeam, handleError])

  useEffect(() => {
    void loadAll()
  }, [loadAll])

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!artifact || !currentTeam) return
    try {
      setUpdating(true)
      await artifactService.updateArtifact(
        currentTeam.id,
        artifact.project_id,
        artifact.slug,
        toArtifactRequest(values)
      )
      trackEvent({
        event: ANALYTICS_EVENTS.ARTIFACT_UPDATED,
        properties: {
          artifact_id: artifact.slug,
          artifact_type: artifact.type,
          artifact_title: artifact.title,
          action_context: 'update',
        },
      })
      showSuccess('Artifact updated successfully', 'Success')
      void navigate(
        `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
      )
    } catch (err) {
      handleError(err, 'Failed to update artifact')
    } finally {
      setUpdating(false)
    }
  }

  // Loading and not-found render in the reading shell too, so the layout is
  // in place before the fetch resolves rather than arriving with the data.
  if (isLoadingTeam || loading) {
    return (
      <ReadingPage title="Loading artifact…" presentation="editing">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ReadingPage>
    )
  }

  if (error || !artifact) {
    return (
      <ReadingPage title="Artifact not found" presentation="editing">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load artifact</AlertTitle>
          <AlertDescription>
            {error ?? 'The artifact could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          className="mt-6"
          onClick={() => {
            void navigate('/artifacts')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to artifacts
        </Button>
      </ReadingPage>
    )
  }

  return (
    <ResourceFormReadingPage
      title={formHeading(descriptor, 'edit')}
      description={artifact.title}
      descriptor={descriptor}
      mode="edit"
      // The fetched resource IS a value map: `defaultFormValues` reads only
      // the keys the descriptor declares, so there is nothing to map here and
      // nothing to keep in sync when a field is added.
      initialValues={artifact}
      onSubmit={handleSubmit}
      isLoading={updating}
      onCancel={() => {
        void navigate(
          `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
        )
      }}
    />
  )
}
