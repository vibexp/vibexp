import { FileText, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListPage, ListTable } from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { PromptFilters, type SharedFilter } from '@/pages/prompts/PromptFilters'
import { buildPromptsColumns } from '@/pages/prompts/promptsColumns'
import { promptService } from '@/services/promptService'
import type { Prompt, PromptFilters as PromptFiltersType } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

type PromptSortKey = NonNullable<PromptFiltersType['sort_by']>

// Backend also accepts 'created_at' as a sort field, but the UI only exposes
// the three columns rendered with sortable headers (name, status, updated_at).
const PROMPT_SORTABLE_KEYS: readonly PromptSortKey[] = [
  'name',
  'status',
  'updated_at',
]

interface State {
  prompts: Prompt[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Prompts() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [state, setState] = useState<State>({
    prompts: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })
  const [filters, setFilters] = useState<PromptFiltersType>({
    search: '',
    sort_by: 'updated_at',
    sort_order: 'desc',
    page: 1,
    limit: 20,
  })
  const [searchInput, setSearchInput] = useState('')
  const [promptToDelete, setPromptToDelete] = useState<Prompt | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchPrompts = useCallback(
    async (current: PromptFiltersType) => {
      if (!currentTeam) return
      setState(prev => ({ ...prev, loading: true, error: null }))
      const response = await promptService.getPrompts(currentTeam.id, current)
      const responseData = 'data' in response ? response.data : response
      const prompts = Array.isArray(responseData.prompts)
        ? responseData.prompts
        : []
      setState({
        prompts,
        loading: false,
        error: null,
        totalPages: responseData.total_pages,
        currentPage: current.page ?? 1,
        total: responseData.total_count,
      })
    },
    [currentTeam]
  )

  useEffect(() => {
    fetchPrompts(filters).catch((error: unknown) => {
      setState(prev => ({
        ...prev,
        loading: false,
        error: getErrorMessage(error, 'Failed to fetch prompts'),
      }))
      handleError(error, 'Failed to load prompts')
    })
  }, [fetchPrompts, filters, handleError])

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
      event: ANALYTICS_EVENTS.PROMPTS_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!promptToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await promptService.deletePrompt(currentTeam.id, promptToDelete.slug)
      showSuccess('Prompt deleted successfully', 'Success')
      void fetchPrompts(filters)
    } catch (error) {
      handleError(error, 'Failed to delete prompt')
    } finally {
      setDeleting(false)
      setPromptToDelete(null)
    }
  }

  const statusFilter = filters.status ?? 'all'
  const sharedFilter: SharedFilter =
    filters.shared === undefined
      ? 'all'
      : filters.shared
        ? 'shared'
        : 'not_shared'

  const sortKey: PromptSortKey = filters.sort_by ?? 'updated_at'
  const sortDir = filters.sort_order ?? 'desc'

  const columns = useMemo(
    () => buildPromptsColumns({ navigate, onDelete: setPromptToDelete }),
    [navigate]
  )

  // Toggle direction when re-clicking the active column; otherwise switch
  // column and pick a sensible default direction (asc for name, desc otherwise).
  const handleSortChange = useCallback((key: PromptSortKey) => {
    setFilters(prev => {
      const prevKey = prev.sort_by ?? 'updated_at'
      const prevDir = prev.sort_order ?? 'desc'
      if (prevKey === key) {
        return {
          ...prev,
          sort_order: prevDir === 'asc' ? 'desc' : 'asc',
          page: 1,
        }
      }
      return {
        ...prev,
        sort_by: key,
        sort_order: key === 'name' ? 'asc' : 'desc',
        page: 1,
      }
    })
  }, [])

  const visibleCount = state.prompts.length
  const totalCount = state.total
  const currentPage = filters.page ?? 1
  const status = state.loading
    ? 'loading'
    : state.error
      ? 'error'
      : state.prompts.length === 0
        ? 'empty'
        : 'ready'

  return (
    <ListPage>
      <ListPage.Header
        title="Prompts"
        description="Organize and manage your AI prompts."
        actions={
          <Button
            onClick={() => {
              // Editor still lives in v1 until Slice 5b lands
              void navigate('/prompts/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New prompt
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <PromptFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            statusFilter={statusFilter}
            onStatusChange={v => {
              setFilters(prev => ({
                ...prev,
                status: v === 'all' ? undefined : v,
                page: 1,
              }))
            }}
            sharedFilter={sharedFilter}
            onSharedChange={v => {
              setFilters(prev => ({
                ...prev,
                shared: v === 'all' ? undefined : v === 'shared',
                page: 1,
              }))
            }}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load prompts"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={FileText}
              title={
                filters.search
                  ? 'No prompts match your filters'
                  : 'No prompts yet'
              }
              description={
                filters.search
                  ? 'Try different search or filter settings.'
                  : 'Create your first prompt to build a reusable AI workflow.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/prompts/new')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  New prompt
                </Button>
              }
            />
          }
        >
          <ListTable
            rows={state.prompts}
            columns={columns}
            sortableKeys={PROMPT_SORTABLE_KEYS}
            sortKey={sortKey}
            sortDir={sortDir}
            onSortChange={handleSortChange}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : { visible: visibleCount, total: totalCount, noun: 'prompt' }
          }
          pagination={{
            page: currentPage,
            totalPages: state.totalPages,
            onPageChange: page => {
              setFilters(prev => ({ ...prev, page }))
            },
          }}
          hideCount={status === 'loading'}
        />
      </ListPage.Container>

      <ConfirmDialog
        open={!!promptToDelete}
        onOpenChange={open => {
          if (!open) setPromptToDelete(null)
        }}
        title="Delete prompt?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {promptToDelete?.name ?? 'this prompt'}
            </span>
            . This action cannot be undone.
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
