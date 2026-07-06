import { ArrowLeft, Save } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  BlueprintForm,
  type BlueprintFormHandle,
} from '@/pages/blueprints/BlueprintForm'
import type {
  CreateBlueprintRequest,
  UpdateBlueprintRequest,
} from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function BlueprintCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess, showError } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [projects, setProjects] = useState<Project[]>([])
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [creating, setCreating] = useState(false)
  const formRef = useRef<BlueprintFormHandle>(null)

  const fetchProjects = useCallback(async () => {
    if (!currentTeam) {
      setLoadingProjects(false)
      return
    }
    try {
      setLoadingProjects(true)
      const res = await projectService.getProjects(currentTeam.id, {
        limit: 100,
      })
      setProjects(res.projects)
    } catch (error) {
      console.error('Failed to fetch projects:', error)
      setProjects([])
    } finally {
      setLoadingProjects(false)
    }
  }, [currentTeam])

  useEffect(() => {
    void fetchProjects()
  }, [fetchProjects])

  const handleSubmit = async (
    data: CreateBlueprintRequest | UpdateBlueprintRequest
  ) => {
    if (!currentTeam) {
      showError('Team context is required', 'Create Failed')
      return
    }
    try {
      setCreating(true)
      const blueprint = await blueprintService.createBlueprint(
        currentTeam.id,
        data as CreateBlueprintRequest
      )
      trackEvent({
        event: ANALYTICS_EVENTS.BLUEPRINT_CREATED,
        properties: {
          blueprint_id: blueprint.slug,
          blueprint_type: blueprint.type,
          blueprint_title: blueprint.title,
          action_context: 'create',
        },
      })
      showSuccess('Blueprint created successfully')
      void navigate(
        `/blueprints/${encodeURIComponent(blueprint.project_id)}/${encodeURIComponent(blueprint.slug)}`
      )
    } catch (error) {
      handleError(error, getErrorMessage(error, 'Failed to create blueprint'))
    } finally {
      setCreating(false)
    }
  }

  if (loadingProjects) {
    return (
      <div className="space-y-6">
        <PageHeader title="Create blueprint" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Create blueprint"
        description="Save AI-generated content to reuse later."
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
              disabled={creating}
            >
              <Save className="mr-2 size-4" />
              {creating ? 'Creating…' : 'Create blueprint'}
            </Button>
          </>
        }
      />
      <BlueprintForm
        ref={formRef}
        projects={projects}
        onSubmit={handleSubmit}
        isLoading={creating}
      />
    </div>
  )
}
