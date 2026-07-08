import { ArrowLeft, Save } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  ArtifactForm,
  type ArtifactFormHandle,
} from '@/pages/artifacts/ArtifactForm'
import type {
  CreateArtifactRequest,
  UpdateArtifactRequest,
} from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function ArtifactCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess, showError } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [creating, setCreating] = useState(false)
  const formRef = useRef<ArtifactFormHandle>(null)

  const handleSubmit = async (
    data: CreateArtifactRequest | UpdateArtifactRequest
  ) => {
    if (!currentTeam) {
      showError('Team context is required', 'Create Failed')
      return
    }
    try {
      setCreating(true)
      const artifact = await artifactService.createArtifact(
        currentTeam.id,
        data as CreateArtifactRequest
      )
      trackEvent({
        event: ANALYTICS_EVENTS.ARTIFACT_CREATED,
        properties: {
          artifact_id: artifact.slug,
          artifact_type: artifact.type,
          artifact_title: artifact.title,
          action_context: 'create',
        },
      })
      showSuccess('Artifact created successfully')
      void navigate(
        `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
      )
    } catch (error) {
      handleError(error, getErrorMessage(error, 'Failed to create artifact'))
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Create artifact"
        description="Save AI-generated content to reuse later."
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
              disabled={creating}
            >
              <Save className="mr-2 size-4" />
              {creating ? 'Creating…' : 'Create artifact'}
            </Button>
          </>
        }
      />
      <ArtifactForm
        ref={formRef}
        onSubmit={handleSubmit}
        isLoading={creating}
      />
    </div>
  )
}
