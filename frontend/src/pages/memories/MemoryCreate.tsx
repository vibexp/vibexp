import { ArrowLeft, Save } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { MemoryForm, type MemoryFormHandle } from '@/pages/memories/MemoryForm'
import type {
  CreateMemoryRequest,
  UpdateMemoryRequest,
} from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

export function MemoryCreate() {
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [projects, setProjects] = useState<Project[]>([])
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [creating, setCreating] = useState(false)
  const formRef = useRef<MemoryFormHandle>(null)

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

  const handleSubmit = async (
    data: CreateMemoryRequest | UpdateMemoryRequest
  ) => {
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
        data as CreateMemoryRequest
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
        <PageHeader title="Create memory" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Create memory"
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
              {creating ? 'Creating…' : 'Create memory'}
            </Button>
          </>
        }
      />
      <MemoryForm
        ref={formRef}
        projects={projects}
        onSubmit={handleSubmit}
        isLoading={creating}
      />
    </div>
  )
}
