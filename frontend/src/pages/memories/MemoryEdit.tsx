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
import { MemoryForm, type MemoryFormHandle } from '@/pages/memories/MemoryForm'
import { memoryService } from '@/services/memoryService'
import { projectService } from '@/services/projectService'
import type {
  CreateMemoryRequest,
  Memory,
  Project,
  UpdateMemoryRequest,
} from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function MemoryEdit() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [memory, setMemory] = useState<Memory | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)
  const formRef = useRef<MemoryFormHandle>(null)

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
      const [response, projectsRes] = await Promise.all([
        memoryService.getMemory(currentTeam.id, id),
        projectService.getProjects(currentTeam.id, { limit: 100 }),
      ])
      setMemory(response)
      setProjects(projectsRes.projects)
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

  const handleSubmit = async (
    data: CreateMemoryRequest | UpdateMemoryRequest
  ) => {
    if (!id || !memory || !currentTeam) return
    try {
      setUpdating(true)
      await memoryService.updateMemory(currentTeam.id, id, data)
      trackEvent({
        event: ANALYTICS_EVENTS.MEMORY_UPDATED,
        properties: {
          memory_id: memory.id,
          memory_type:
            typeof memory.metadata.type === 'string'
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

  if (isLoadingTeam || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading memory…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !memory) {
    return (
      <div className="space-y-6">
        <PageHeader title="Memory not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load memory</AlertTitle>
          <AlertDescription>
            {error ?? 'The memory could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/memories')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to memories
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Edit memory"
        description="Update the content or tags."
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
              disabled={updating}
            >
              <Save className="mr-2 size-4" />
              {updating ? 'Saving…' : 'Save changes'}
            </Button>
          </>
        }
      />
      <MemoryForm
        ref={formRef}
        memory={memory}
        projects={projects}
        onSubmit={handleSubmit}
        isLoading={updating}
      />
    </div>
  )
}
