import { ArrowLeft, Save } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
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
  UpdateProjectRequest,
} from '@/services/projectService'
import { projectService } from '@/services/projectService'

export function ProjectCreate() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess, showError } = useAlerts()
  const { handleError } = useErrorHandler()
  const [creating, setCreating] = useState(false)
  const formRef = useRef<ProjectFormHandle>(null)

  const handleSubmit = async (
    data: CreateProjectRequest | UpdateProjectRequest
  ) => {
    if (!currentTeam) {
      showError('Team context is required', 'Create Failed')
      return
    }
    try {
      setCreating(true)
      await projectService.createProject(
        currentTeam.id,
        data as CreateProjectRequest
      )
      showSuccess('Project created successfully', 'Success')
      void navigate('/settings/projects')
    } catch (error) {
      handleError(error, 'Failed to create project')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Create project"
        description="Organize artifacts and blueprints inside a project."
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
              disabled={creating}
            >
              <Save className="mr-2 size-4" />
              {creating ? 'Creating…' : 'Create project'}
            </Button>
          </>
        }
      />
      <ProjectForm ref={formRef} onSubmit={handleSubmit} isLoading={creating} />
    </div>
  )
}
