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
import {
  requiredMetadataKeys,
  toBlueprintRequest,
} from '@/pages/blueprints/blueprintRequest'
import type { Blueprint } from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const descriptor = getResourceDescriptor('blueprint')

export function BlueprintEdit() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [blueprint, setBlueprint] = useState<Blueprint | null>(null)
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
      const a = await blueprintService.getBlueprint(
        currentTeam.id,
        decodeURIComponent(project),
        decodeURIComponent(slug)
      )
      setBlueprint(a)
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to load blueprint'))
      handleError(err, 'Failed to load blueprint')
    } finally {
      setLoading(false)
    }
  }, [project, slug, currentTeam, isLoadingTeam, handleError])

  useEffect(() => {
    void loadAll()
  }, [loadAll])

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!blueprint || !currentTeam) return
    try {
      setUpdating(true)
      await blueprintService.updateBlueprint(
        currentTeam.id,
        blueprint.project_id,
        blueprint.slug,
        toBlueprintRequest(values)
      )
      trackEvent({
        event: ANALYTICS_EVENTS.BLUEPRINT_UPDATED,
        properties: {
          blueprint_id: blueprint.slug,
          blueprint_type: blueprint.type,
          blueprint_title: blueprint.title,
          action_context: 'update',
        },
      })
      showSuccess('Blueprint updated successfully', 'Success')
      void navigate(
        `/blueprints/${encodeURIComponent(blueprint.project_id)}/${encodeURIComponent(blueprint.slug)}`
      )
    } catch (err) {
      handleError(err, 'Failed to update blueprint')
    } finally {
      setUpdating(false)
    }
  }

  // Loading and not-found render in the reading shell too, so the layout is
  // in place before the fetch resolves rather than arriving with the data.
  if (isLoadingTeam || loading) {
    return (
      <ReadingPage title="Loading blueprint…" presentation="editing">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ReadingPage>
    )
  }

  if (error || !blueprint) {
    return (
      <ReadingPage title="Blueprint not found" presentation="editing">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load blueprint</AlertTitle>
          <AlertDescription>
            {error ?? 'The blueprint could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          className="mt-6"
          onClick={() => {
            void navigate('/blueprints')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to blueprints
        </Button>
      </ReadingPage>
    )
  }

  return (
    <ResourceFormReadingPage
      title={formHeading(descriptor, 'edit')}
      description={blueprint.title}
      descriptor={descriptor}
      mode="edit"
      initialValues={blueprint}
      onSubmit={handleSubmit}
      isLoading={updating}
      metadataRequiredKeys={requiredMetadataKeys(blueprint)}
      onCancel={() => {
        void navigate(
          `/blueprints/${encodeURIComponent(blueprint.project_id)}/${encodeURIComponent(blueprint.slug)}`
        )
      }}
    />
  )
}
