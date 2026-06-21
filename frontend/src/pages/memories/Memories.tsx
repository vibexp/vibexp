import { HardDrive, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListPage, ListTable } from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import {
  buildMemoriesColumns,
  extractTags,
} from '@/pages/memories/memoriesColumns'
import { MemoryFilters } from '@/pages/memories/MemoryFilters'
import { memoryService } from '@/services/memoryService'
import { projectService } from '@/services/projectService'
import type {
  Memory,
  MemoryFilters as MemoryFiltersType,
  Project,
} from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

interface MemoriesState {
  memories: Memory[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Memories() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [state, setState] = useState<MemoriesState>({
    memories: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })

  const [filters, setFilters] = useState<MemoryFiltersType>({
    search: '',
    sort_by: 'updated_at',
    sort_order: 'desc',
    page: 1,
    limit: 20,
  })
  const [searchInput, setSearchInput] = useState('')
  const [selectedTag, setSelectedTag] = useState<string | undefined>()
  const [projects, setProjects] = useState<Project[]>([])
  const [memoryToDelete, setMemoryToDelete] = useState<Memory | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchMemories = useCallback(
    async (currentFilters: MemoryFiltersType) => {
      if (!currentTeam) return
      setState(prev => ({ ...prev, loading: true, error: null }))
      const response = await memoryService.getMemories(
        currentTeam.id,
        currentFilters
      )
      const memories = Array.isArray(response.memories) ? response.memories : []
      setState(prev => ({
        ...prev,
        memories,
        totalPages: response.total_pages,
        currentPage: currentFilters.page ?? 1,
        total: response.total_count,
        loading: false,
      }))
    },
    [currentTeam]
  )

  useEffect(() => {
    fetchMemories(filters).catch((error: unknown) => {
      const errorMessage = getErrorMessage(error, 'Failed to fetch memories')
      setState(prev => ({ ...prev, loading: false, error: errorMessage }))
      handleError(error, 'Failed to load memories')
    })
  }, [fetchMemories, filters, handleError])

  useEffect(() => {
    const fetchProjects = async () => {
      if (!currentTeam) return
      try {
        const res = await projectService.getProjects(currentTeam.id, {
          limit: 100,
        })
        setProjects(res.projects)
      } catch (error) {
        handleError(error, 'Failed to load projects')
      }
    }
    void fetchProjects()
  }, [currentTeam, handleError])

  useEffect(() => {
    const timeout = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : { ...prev, search: searchInput, page: 1 }
      )
    }, 500)
    return () => {
      clearTimeout(timeout)
    }
  }, [searchInput])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.MEMORIES_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!memoryToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await memoryService.deleteMemory(currentTeam.id, memoryToDelete.id)
      void fetchMemories(filters)
      showSuccess('Memory deleted successfully', 'Success')
    } catch (error) {
      handleError(error, 'Failed to delete memory')
    } finally {
      setDeleting(false)
      setMemoryToDelete(null)
    }
  }

  const hasAnyTags = useMemo(
    () => state.memories.some(m => extractTags(m.metadata).length > 0),
    [state.memories]
  )

  const allTags = useMemo(
    () =>
      Array.from(
        new Set(state.memories.flatMap(m => extractTags(m.metadata)))
      ).sort(),
    [state.memories]
  )

  useEffect(() => {
    if (selectedTag && !allTags.includes(selectedTag)) {
      setSelectedTag(undefined)
    }
  }, [allTags, selectedTag])

  // Clear stale project filter when team changes and the project is no longer in scope
  useEffect(() => {
    if (
      filters.project_id &&
      projects.length > 0 &&
      !projects.some(p => p.id === filters.project_id)
    ) {
      setFilters(prev => ({ ...prev, project_id: undefined, page: 1 }))
    }
  }, [projects, filters.project_id])

  const displayedMemories = useMemo(
    () =>
      selectedTag
        ? state.memories.filter(m =>
            extractTags(m.metadata).includes(selectedTag)
          )
        : state.memories,
    [state.memories, selectedTag]
  )

  const sortKey = filters.sort_by ?? 'updated_at'
  const sortDir = filters.sort_order ?? 'desc'

  const handleSortChange = useCallback((key: 'updated_at') => {
    setFilters(prev => {
      if (key === prev.sort_by) {
        return {
          ...prev,
          sort_order: prev.sort_order === 'asc' ? 'desc' : 'asc',
          page: 1,
        }
      }
      return { ...prev, sort_by: key, sort_order: 'desc', page: 1 }
    })
  }, [])

  const selectedProject = useMemo(
    () => projects.find(p => p.id === filters.project_id),
    [projects, filters.project_id]
  )

  const columns = useMemo(
    () =>
      buildMemoriesColumns({
        navigate,
        onDelete: setMemoryToDelete,
        includeTags: hasAnyTags,
        projects,
      }),
    [navigate, hasAnyTags, projects]
  )

  const status = state.loading
    ? 'loading'
    : state.error
      ? 'error'
      : state.memories.length === 0
        ? 'empty'
        : 'ready'

  return (
    <ListPage>
      <ListPage.Header
        title="Memories"
        description="Browse and manage AI memories."
        actions={
          <Button
            onClick={() => {
              void navigate('/memories/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New memory
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <MemoryFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            tags={allTags}
            selectedTag={selectedTag}
            onTagChange={setSelectedTag}
            projects={projects}
            selectedProjectId={filters.project_id}
            onProjectChange={value => {
              setFilters(prev => ({ ...prev, project_id: value, page: 1 }))
            }}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load memories"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={HardDrive}
              title={
                filters.search || filters.project_id
                  ? 'No memories match your filters'
                  : 'No memories yet'
              }
              description={
                filters.search && filters.project_id
                  ? 'Try a different search term or clear the filters.'
                  : filters.project_id && selectedProject
                    ? `No memories in ${selectedProject.name}. Create one to get started.`
                    : filters.search || filters.project_id
                      ? 'Try a different search term or clear the filter.'
                      : 'Create your first memory to save insights, snippets, or notes.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/memories/new')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  New memory
                </Button>
              }
            />
          }
        >
          <ListTable
            rows={displayedMemories}
            columns={columns}
            sortableKeys={['updated_at'] as const}
            sortKey={sortKey}
            sortDir={sortDir}
            onSortChange={handleSortChange}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : {
                  visible: displayedMemories.length,
                  total: state.total,
                  noun: 'memory',
                  nounPlural: 'memories',
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
        open={!!memoryToDelete}
        onOpenChange={open => {
          if (!open) setMemoryToDelete(null)
        }}
        title="Delete memory?"
        description="This will permanently delete the memory. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </ListPage>
  )
}
