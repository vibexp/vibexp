import { FolderKanban, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListPage, ListTable } from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { ProjectFilters } from '@/pages/settings/projects/ProjectFilters'
import { buildProjectsColumns } from '@/pages/settings/projects/projectsColumns'
import { projectService } from '@/services/projectService'
import type {
  Project,
  ProjectFilters as ProjectFiltersType,
} from '@/types/project'
import { getErrorMessage } from '@/utils/errorHandling'

interface State {
  projects: Project[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Projects() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [state, setState] = useState<State>({
    projects: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })
  const [filters, setFilters] = useState<ProjectFiltersType>({
    search: '',
    page: 1,
    limit: 20,
    sort_by: 'updated_at',
    sort_order: 'desc',
  })
  const [searchInput, setSearchInput] = useState('')
  const [projectToDelete, setProjectToDelete] = useState<Project | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchProjects = useCallback(
    async (current: ProjectFiltersType) => {
      if (!currentTeam) return
      try {
        setState(prev => ({ ...prev, loading: true, error: null }))
        const response = await projectService.getProjects(
          currentTeam.id,
          current
        )
        setState({
          projects: response.projects,
          loading: false,
          error: null,
          totalPages: response.total_pages,
          currentPage: current.page ?? 1,
          total: response.total_count,
        })
      } catch (error) {
        setState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(error, 'Failed to fetch projects'),
        }))
        handleError(error, 'Failed to load projects')
      }
    },
    [currentTeam, handleError]
  )

  useEffect(() => {
    void fetchProjects(filters)
  }, [fetchProjects, filters])

  useEffect(() => {
    const t = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : { ...prev, search: searchInput, page: 1 }
      )
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput])

  const handleDelete = async () => {
    if (!projectToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await projectService.deleteProject(currentTeam.id, projectToDelete.slug)
      showSuccess('Project deleted successfully', 'Success')
      void fetchProjects(filters)
    } catch (error) {
      handleError(error, 'Failed to delete project')
    } finally {
      setDeleting(false)
      setProjectToDelete(null)
    }
  }

  const columns = useMemo(
    () => buildProjectsColumns({ navigate, onDelete: setProjectToDelete }),
    [navigate]
  )

  const status = state.loading
    ? 'loading'
    : state.error
      ? 'error'
      : state.projects.length === 0
        ? 'empty'
        : 'ready'

  return (
    <ListPage>
      <ListPage.Header
        title="Projects"
        description="Organize your work into projects."
        actions={
          <Button
            onClick={() => {
              void navigate('/settings/projects/create')
            }}
          >
            <Plus className="mr-2 size-4" />
            New project
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <ProjectFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load projects"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={FolderKanban}
              title={
                filters.search
                  ? 'No projects match your search'
                  : 'No projects yet'
              }
              description={
                filters.search
                  ? 'Try a different search term.'
                  : 'Create your first project to start organizing your work.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/settings/projects/create')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  New project
                </Button>
              }
            />
          }
        >
          <ListTable rows={state.projects} columns={columns} />
        </ListPage.Body>

        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : {
                  visible: state.projects.length,
                  total: state.total,
                  noun: 'project',
                }
          }
          pagination={{
            page: state.currentPage,
            totalPages: state.totalPages,
            onPageChange: page => {
              setFilters(prev => ({ ...prev, page }))
            },
          }}
          hideCount={status === 'loading'}
        />
      </ListPage.Container>

      <ConfirmDialog
        open={!!projectToDelete}
        onOpenChange={open => {
          if (!open) setProjectToDelete(null)
        }}
        title="Delete project?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {projectToDelete?.name ?? 'this project'}
            </span>
            . Artifacts and blueprints tied to this project may also be
            affected. This action cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </ListPage>
  )
}
