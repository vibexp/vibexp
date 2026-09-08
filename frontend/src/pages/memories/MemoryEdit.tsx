import { AlertCircle, ArrowLeft } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
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
  extractTags,
  memoryInitialValues,
  RESERVED_METADATA_KEYS,
  toMemoryRequest,
} from '@/pages/memories/memoryRequest'
import { MemoryTagsCard } from '@/pages/memories/MemoryTagsCard'
import type { Memory } from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const descriptor = getResourceDescriptor('memory')

/** Order-sensitive: the tag list is a sequence the reader can reorder. */
function sameTags(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((tag, index) => tag === b[index])
}

export function MemoryEdit() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [memory, setMemory] = useState<Memory | null>(null)
  const [tags, setTags] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)

  const loadAll = useCallback(async () => {
    if (isLoadingTeam) return
    if (!id) {
      setError('Memory ID is required')
      setLoading(false)
      return
    }
    if (!currentTeam) {
      setError('Team context is required')
      setLoading(false)
      return
    }
    try {
      setLoading(true)
      setError(null)
      const response = await memoryService.getMemory(currentTeam.id, id)
      setMemory(response)
      setTags(extractTags(response.metadata))
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to load memory'))
      handleError(err, 'Failed to load memory')
    } finally {
      setLoading(false)
    }
  }, [id, currentTeam, isLoadingTeam, handleError])

  useEffect(() => {
    void loadAll()
  }, [loadAll])

  const handleSubmit = async (values: ResourceFormValues) => {
    if (!id || !memory || !currentTeam) return
    try {
      setUpdating(true)
      // `null`, not omitted: on an update the API reads a missing `title` as
      // "leave it alone" and `null` as "clear it", so emptying the field must
      // send the explicit null.
      await memoryService.updateMemory(
        currentTeam.id,
        id,
        toMemoryRequest(values, tags, 'clear')
      )
      trackEvent({
        event: ANALYTICS_EVENTS.MEMORY_UPDATED,
        properties: {
          memory_id: memory.id,
          memory_type:
            typeof memory.metadata?.type === 'string'
              ? memory.metadata.type
              : 'unknown',
          action_context: 'update',
        },
      })
      showSuccess('Memory updated successfully', 'Success')
      void navigate('/memories')
    } catch (err) {
      handleError(err, 'Failed to update memory')
      throw err
    } finally {
      setUpdating(false)
    }
  }

  // Memoised on the resource, not rebuilt per render: `ResourceFormPage`
  // re-seeds on a CONTENT change, so a fresh literal is harmless but a fresh
  // `JSON.stringify` of the whole memory on every keystroke is not.
  const initialValues = useMemo(
    () => (memory ? memoryInitialValues(memory) : undefined),
    [memory]
  )

  // Loading and not-found render in the reading shell too, so the layout is
  // in place before the fetch resolves rather than arriving with the data.
  if (isLoadingTeam || loading) {
    return (
      <ReadingPage title="Loading memory…" presentation="editing">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ReadingPage>
    )
  }

  if (error || !memory) {
    return (
      // A terminal error state, not a page waiting on a fetch: there is no
      // form coming, so the editing presentation would only add an empty
      // details column and a toggle that opens nothing.
      <ReadingPage title="Memory not found">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load memory</AlertTitle>
          <AlertDescription>
            {error ?? 'The memory could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          className="mt-6"
          onClick={() => {
            void navigate('/memories')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to memories
        </Button>
      </ReadingPage>
    )
  }

  return (
    <ResourceFormReadingPage
      title={formHeading(descriptor, 'edit')}
      description="Update the content or tags."
      descriptor={descriptor}
      mode="edit"
      initialValues={initialValues}
      onSubmit={handleSubmit}
      isLoading={updating}
      metadataReservedKeys={RESERVED_METADATA_KEYS}
      // The tags card is page state, invisible to react-hook-form: without
      // this, adding a tag and hitting Cancel discards it with no prompt.
      extraDirty={!sameTags(tags, extractTags(memory.metadata))}
      extensions={{
        tags: (
          <MemoryTagsCard value={tags} onChange={setTags} disabled={updating} />
        ),
      }}
      onCancel={() => {
        void navigate(`/memories/${memory.id}`)
      }}
    />
  )
}
