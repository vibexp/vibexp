import { ArrowLeft, Save } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import type {
  ResourceFormHandle,
  ResourceFormValues,
} from '@/components/patterns/resource'
import {
  formHeading,
  formSaveLabel,
  getResourceDescriptor,
  ResourceFormPage,
} from '@/components/patterns/resource'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toBlueprintRequest } from '@/pages/blueprints/blueprintRequest'
import { blueprintService } from '@/services/blueprintService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const descriptor = getResourceDescriptor('blueprint')

export function BlueprintCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess, showError } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [creating, setCreating] = useState(false)
  const formRef = useRef<ResourceFormHandle>(null)

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!currentTeam) {
      showError('Team context is required', 'Create Failed')
      return
    }
    try {
      setCreating(true)
      const blueprint = await blueprintService.createBlueprint(
        currentTeam.id,
        toBlueprintRequest(values)
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

  return (
    <div className="space-y-6">
      <PageHeader
        title={formHeading(descriptor, 'create')}
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
              {creating ? 'Creating…' : formSaveLabel(descriptor, 'create')}
            </Button>
          </>
        }
      />
      <ResourceFormPage
        ref={formRef}
        descriptor={descriptor}
        mode="create"
        onSubmit={handleSubmit}
        isLoading={creating}
      />
    </div>
  )
}
