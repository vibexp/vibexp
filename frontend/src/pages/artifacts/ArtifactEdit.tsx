import { AlertCircle, ArrowLeft, Save } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  ArtifactForm,
  type ArtifactFormHandle,
} from '@/pages/artifacts/ArtifactForm'
import type {
  Artifact,
  CreateArtifactRequest,
  UpdateArtifactRequest,
} from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

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
  const formRef = useRef<ArtifactFormHandle>(null)

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

  const handleSubmit = async (
    data: CreateArtifactRequest | UpdateArtifactRequest
  ) => {
    if (!artifact || !currentTeam) return
    try {
      setUpdating(true)
      await artifactService.updateArtifact(
        currentTeam.id,
        artifact.project_id,
        artifact.slug,
        data
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

  if (isLoadingTeam || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading artifact…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !artifact) {
    return (
      <div className="space-y-6">
        <PageHeader title="Artifact not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load artifact</AlertTitle>
          <AlertDescription>
            {error ?? 'The artifact could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/artifacts')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to artifacts
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Edit artifact"
        description={artifact.title}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/artifacts')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <Button
              onClick={() => {
                formRef.current?.submit()
              }}
              disabled={updating}
            >
              <Save className="mr-2 size-4" />
              {updating ? 'Saving…' : 'Save changes'}
            </Button>
          </>
        }
      />
      <ArtifactForm
        ref={formRef}
        artifact={artifact}
        onSubmit={handleSubmit}
        isLoading={updating}
      />
    </div>
  )
}
