import { AlertCircle, ArrowLeft, Save } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  ProjectForm,
  type ProjectFormHandle,
} from '@/pages/settings/projects/ProjectForm'
import type {
  CreateProjectRequest,
  Project,
  UpdateProjectRequest,
} from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { getErrorMessage } from '@/utils/errorHandling'

export function ProjectEdit() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [project, setProject] = useState<Project | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)
  const formRef = useRef<ProjectFormHandle>(null)

  useEffect(() => {
    const load = async () => {
      if (isLoadingTeam) return
      if (!slug) {
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
        const p = await projectService.getProject(currentTeam.id, slug)
        setProject(p)
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to load project'))
        handleError(err, 'Failed to load project')
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [slug, currentTeam, isLoadingTeam, handleError])

  const handleSubmit = async (
    data: CreateProjectRequest | UpdateProjectRequest
  ) => {
    if (!project || !currentTeam) return
    try {
      setUpdating(true)
      await projectService.updateProject(currentTeam.id, project.slug, data)
      showSuccess('Project updated successfully', 'Success')
      void navigate('/settings/projects')
    } catch (err) {
      handleError(err, 'Failed to update project')
    } finally {
      setUpdating(false)
    }
  }

  if (isLoadingTeam || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading project…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !project) {
    return (
      <div className="space-y-6">
        <PageHeader title="Project not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load project</AlertTitle>
          <AlertDescription>
            {error ?? 'The project could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/settings/projects')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Edit project"
        description={project.name}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/settings/projects')
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
      <ProjectForm
        ref={formRef}
        project={project}
        onSubmit={handleSubmit}
        isLoading={updating}
      />
    </div>
  )
}
