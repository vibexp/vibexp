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
  BlueprintForm,
  type BlueprintFormHandle,
} from '@/pages/blueprints/BlueprintForm'
import { blueprintService } from '@/services/blueprintService'
import { projectService } from '@/services/projectService'
import type {
  Blueprint,
  CreateBlueprintRequest,
  Project,
  UpdateBlueprintRequest,
} from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function BlueprintEdit() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [blueprint, setBlueprint] = useState<Blueprint | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)
  const formRef = useRef<BlueprintFormHandle>(null)

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
      const [a, projectsRes] = await Promise.all([
        blueprintService.getBlueprint(
          currentTeam.id,
          decodeURIComponent(project),
          decodeURIComponent(slug)
        ),
        projectService.getProjects(currentTeam.id, { limit: 100 }),
      ])
      setBlueprint(a)
      setProjects(projectsRes.projects)
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

  const handleSubmit = async (
    data: CreateBlueprintRequest | UpdateBlueprintRequest
  ) => {
    if (!blueprint || !currentTeam) return
    try {
      setUpdating(true)
      await blueprintService.updateBlueprint(
        currentTeam.id,
        blueprint.project_id,
        blueprint.slug,
        data
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

  if (isLoadingTeam || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading blueprint…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !blueprint) {
    return (
      <div className="space-y-6">
        <PageHeader title="Blueprint not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load blueprint</AlertTitle>
          <AlertDescription>
            {error ?? 'The blueprint could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/blueprints')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to blueprints
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Edit blueprint"
        description={blueprint.title}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/blueprints')
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
      <BlueprintForm
        ref={formRef}
        blueprint={blueprint}
        projects={projects}
        onSubmit={handleSubmit}
        isLoading={updating}
      />
    </div>
  )
}
