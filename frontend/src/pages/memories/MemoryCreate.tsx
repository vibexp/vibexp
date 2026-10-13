import { useMemo, useState } from 'react'
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
import { useDefaultProject } from '@/hooks/useDefaultProject'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  RESERVED_METADATA_KEYS,
  toMemoryRequest,
} from '@/pages/memories/memoryRequest'
import { MemoryTagsField } from '@/pages/memories/MemoryTagsField'
import { memoryService } from '@/services/memoryService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const descriptor = getResourceDescriptor('memory')

export function MemoryCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [creating, setCreating] = useState(false)
  const [tags, setTags] = useState<string[]>([])

  // The shared create-form default: the header's project filter, or the
  // workspace's only project. `e2e/memories.spec.ts` creates a memory without
  // touching the picker, which the single-project e2e workspace still allows.
  const defaultProject = useDefaultProject()
  const defaultProjectId = defaultProject.project?.id
  const initialValues = useMemo<ResourceFormValues | undefined>(
    () => (defaultProjectId ? { project_id: defaultProjectId } : undefined),
    [defaultProjectId]
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
  if (defaultProject.loading) {
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
      saveDisabled={!defaultProject.hasProjects}
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
