import { useState } from 'react'
import { useNavigate } from 'react-router'

import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  formHeading,
  getResourceDescriptor,
  ResourceFormReadingPage,
} from '@/components/patterns/resource'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toArtifactRequest } from '@/pages/artifacts/artifactRequest'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const descriptor = getResourceDescriptor('artifact')

/** Stable reference: a fresh literal would re-seed the form on every render. */
const INITIAL_VALUES: ResourceFormValues = { type: 'general' }

export function ArtifactCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess, showError } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [creating, setCreating] = useState(false)

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!currentTeam) {
      showError('Team context is required', 'Create Failed')
      return
    }
    try {
      setCreating(true)
      const artifact = await artifactService.createArtifact(
        currentTeam.id,
        toArtifactRequest(values)
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
    <ResourceFormReadingPage
      title={formHeading(descriptor, 'create')}
      descriptor={descriptor}
      mode="create"
      // `type` reads the team's runtime type catalog, so the generated form
      // cannot pick an opening value for it the way it does for a status —
      // and the artifact e2e journeys that never touch the type select rely
      // on the API's own default being preselected, as the old form did.
      initialValues={INITIAL_VALUES}
      onSubmit={handleSubmit}
      isLoading={creating}
      onCancel={() => {
        void navigate('/artifacts')
      }}
    />
  )
}
