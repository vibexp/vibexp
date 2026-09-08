import { ArrowLeft, Save } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
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
import {
  RESERVED_METADATA_KEYS,
  toMemoryRequest,
} from '@/pages/memories/memoryRequest'
import { MemoryTagsCard } from '@/pages/memories/MemoryTagsCard'
import { memoryService } from '@/services/memoryService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const descriptor = getResourceDescriptor('memory')

export function MemoryCreate() {
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [projects, setProjects] = useState<Project[]>([])
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [creating, setCreating] = useState(false)
  const [tags, setTags] = useState<string[]>([])
  const formRef = useRef<ResourceFormHandle>(null)

  const fetchProjects = useCallback(async () => {
    if (isLoadingTeam) return
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
  }, [currentTeam, isLoadingTeam])

  useEffect(() => {
    void fetchProjects()
  }, [fetchProjects])

  // The one kind whose create page still fetches projects itself: it preselects
  // the first one, which `ProjectPicker` deliberately does not do (#1790). That
  // default is the reason `e2e/memories.spec.ts` can create a memory without
  // touching the picker, so it is behaviour rather than convenience.
  const initialValues = useMemo<ResourceFormValues | undefined>(
    () => (projects.length > 0 ? { project_id: projects[0].id } : undefined),
    [projects]
  )

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!currentTeam) {
      handleError(
        new Error('Team context is required'),
        'Failed to create memory'
      )
      return
    }
    try {
      setCreating(true)
      const memory = await memoryService.createMemory(
        currentTeam.id,
        // A create omits an empty title entirely; only an update sends `null`,
        // which is how the API distinguishes "unchanged" from "cleared".
        toMemoryRequest(values, tags, undefined)
      )
      trackEvent({
        event: ANALYTICS_EVENTS.MEMORY_CREATED,
        properties: {
          memory_id: memory.id,
          memory_type: 'manual',
          action_context: 'create',
        },
      })
      showSuccess('Memory created successfully', 'Success')
      void navigate('/memories')
    } catch (error) {
      handleError(error, 'Failed to create memory')
      throw error
    } finally {
      setCreating(false)
    }
  }

  if (loadingProjects) {
    return (
      <div className="space-y-6">
        <PageHeader title={formHeading(descriptor, 'create')} />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={formHeading(descriptor, 'create')}
        description="Save a new memory for future reference."
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/memories')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <Button
              onClick={() => {
                formRef.current?.submit()
              }}
              disabled={creating || projects.length === 0}
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
        initialValues={initialValues}
        onSubmit={handleSubmit}
        isLoading={creating}
        metadataReservedKeys={RESERVED_METADATA_KEYS}
        extensions={{
          tags: (
            <MemoryTagsCard
              value={tags}
              onChange={setTags}
              disabled={creating}
            />
          ),
        }}
      />
    </div>
  )
}
