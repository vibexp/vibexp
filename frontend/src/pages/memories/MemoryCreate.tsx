import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { ReadingPage } from '@/components/patterns/reading-page'
import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  formHeading,
  getResourceDescriptor,
  ResourceFormReadingPage,
} from '@/components/patterns/resource'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  RESERVED_METADATA_KEYS,
  toMemoryRequest,
} from '@/pages/memories/memoryRequest'
import { MemoryTagsField } from '@/pages/memories/MemoryTagsField'
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
        toMemoryRequest(values, tags, 'omit')
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

  // The skeleton renders in the reading shell too, so the layout is in place
  // before the projects resolve rather than arriving with the form.
  if (loadingProjects) {
    return (
      <ReadingPage
        title={formHeading(descriptor, 'create')}
        presentation="editing"
      >
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ReadingPage>
    )
  }

  return (
    <ResourceFormReadingPage
      title={formHeading(descriptor, 'create')}
      descriptor={descriptor}
      mode="create"
      initialValues={initialValues}
      onSubmit={handleSubmit}
      isLoading={creating}
      // A memory must belong to a project; with none to pick, Create could
      // only ever fail validation.
      saveDisabled={projects.length === 0}
      // The tags are page state, invisible to react-hook-form: without this,
      // adding one and hitting Cancel discards it with no prompt.
      extraDirty={tags.length > 0}
      metadataReservedKeys={RESERVED_METADATA_KEYS}
      extensions={{
        tags: (
          <MemoryTagsField
            value={tags}
            onChange={setTags}
            disabled={creating}
          />
        ),
      }}
      onCancel={() => {
        void navigate('/memories')
      }}
    />
  )
}
