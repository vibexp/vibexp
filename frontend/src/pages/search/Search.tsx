import { Search as SearchIcon } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'

import { EmptyState } from '@/components/EmptyState'
import type { ListPageStatus } from '@/components/patterns/list-page'
import { ListPage } from '@/components/patterns/list-page'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { AiSummary } from '@/pages/search/AiSummary'
import { SearchFilters } from '@/pages/search/SearchFilters'
import { SearchResultCard } from '@/pages/search/searchResult'
import { useSearchSummary } from '@/pages/search/useSearchSummary'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import type {
  SearchAISummaryAvailability,
  SearchFilterType,
  SearchRequest,
  SearchResultItem,
} from '@/services/searchService'
import { searchService } from '@/services/searchService'
import { getErrorMessage } from '@/utils/errorHandling'

const PER_PAGE = 20

// How long a result card stays highlighted after a citation scrolls to it.
const CITATION_HIGHLIGHT_MS = 2000

/** DOM id of the (first) result card for a resource, for citation links. */
function resultCardDomId(resourceId: string): string {
  return `search-result-${resourceId}`
}

const SEARCH_TYPES: SearchFilterType[] = [
  'prompts',
  'artifacts',
  'blueprints',
  'memories',
]

/** Coerce a raw `type` URL param to a known filter value, or undefined. */
function parseType(value: string | null): SearchFilterType | undefined {
  return SEARCH_TYPES.find(t => t === value)
}

interface SearchState {
  results: SearchResultItem[]
  status: ListPageStatus
  error: string | null
  total: number
  totalPages: number
  page: number
  aiSummary?: SearchAISummaryAvailability
}

const INITIAL_STATE: SearchState = {
  results: [],
  status: 'ready',
  error: null,
  total: 0,
  totalPages: 0,
  page: 1,
}

export function Search() {
  const { currentTeam } = useTeam()
  const { handleError } = useErrorHandler()
  const { can } = usePermissions()
  const [searchParams, setSearchParams] = useSearchParams()

  const q = searchParams.get('q') ?? ''
  const page = Number(searchParams.get('page') ?? '1') || 1
  const type = parseType(searchParams.get('type'))
  const projectId = searchParams.get('project') ?? undefined
  const hasQuery = q.trim().length > 0

  const [state, setState] = useState<SearchState>(INITIAL_STATE)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [projects, setProjects] = useState<Project[]>([])
  // Local, uncommitted text in the query box; committed to the `q` param on submit.
  const [queryInput, setQueryInput] = useState(q)

  const [highlightedId, setHighlightedId] = useState<string | null>(null)
  const highlightTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined
  )

  const teamId = currentTeam?.id
  const aiSummary = useSearchSummary(teamId, q, type, projectId)

  // Arriving from the header search dialog's "See full results"
  // (`summary=open`, #1079) opens the AI Summary for that query, reusing the
  // answer the dialog already cached. The param is consumed right away so a
  // refresh or back-navigation follows the user's stored preference again.
  const [preExpandedQuery, setPreExpandedQuery] = useState<string | null>(null)
  const summaryParam = searchParams.get('summary')
  useEffect(() => {
    if (summaryParam === null) return
    if (summaryParam === 'open') setPreExpandedQuery(q)
    setSearchParams(
      prev => {
        const params = new URLSearchParams(prev)
        params.delete('summary')
        return params
      },
      { replace: true }
    )
  }, [summaryParam, q, setSearchParams])

  // Keep the query box in sync when `q` changes externally (e.g. navigating in
  // from the header search modal or the browser back button).
  useEffect(() => {
    setQueryInput(q)
  }, [q])

  // Load the team's projects for the project filter dropdown.
  useEffect(() => {
    if (!teamId) return
    let cancelled = false
    projectService
      .getProjects(teamId, { limit: 100, sort_by: 'name', sort_order: 'asc' })
      .then(res => {
        if (!cancelled) setProjects(res.projects)
      })
      .catch((error: unknown) => {
        if (!cancelled) handleError(error, 'Failed to load projects')
      })
    return () => {
      cancelled = true
    }
  }, [teamId, handleError])

  // Drop a project filter that no longer belongs to the current team's projects
  // (e.g. after switching teams) so we never search by a stale project id.
  useEffect(() => {
    if (
      projectId &&
      projects.length > 0 &&
      !projects.some(p => p.id === projectId)
    ) {
      setSearchParams(prev => {
        const params = new URLSearchParams(prev)
        params.delete('project')
        params.set('page', '1')
        return params
      })
    }
  }, [projectId, projects, setSearchParams])

  useEffect(() => {
    if (!teamId || !hasQuery) return
    // Guard against out-of-order responses: rapid pagination/query/filter changes
    // can resolve an older request after a newer one. The cleanup flag discards any
    // response from a superseded (or unmounted) effect run.
    let cancelled = false
    setExpanded(new Set())
    setState(prev => ({ ...prev, status: 'loading', error: null }))
    const req: SearchRequest = { query: q, page, per_page: PER_PAGE }
    if (type) req.types = [type]
    if (projectId) req.project_id = projectId
    searchService
      .search(teamId, req)
      .then(response => {
        if (cancelled) return
        setState({
          results: response.results,
          status: response.results.length === 0 ? 'empty' : 'ready',
          error: null,
          total: response.total_count,
          totalPages: response.total_pages,
          page: response.page,
          aiSummary: response.ai_summary,
        })
      })
      .catch((error: unknown) => {
        if (cancelled) return
        setState(prev => ({
          ...prev,
          status: 'error',
          error: getErrorMessage(error, 'Failed to search'),
        }))
        handleError(error, 'Failed to search')
      })
    return () => {
      cancelled = true
    }
  }, [teamId, hasQuery, q, page, type, projectId, handleError])

  const toggleExpand = useCallback((chunkId: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(chunkId)) {
        next.delete(chunkId)
      } else {
        next.add(chunkId)
      }
      return next
    })
  }, [])

  // First card per resource id: one resource can yield several chunk cards,
  // and a citation targets the resource, so it lands on the first of them.
  const firstCardIds = useMemo(() => {
    const seen = new Set<string>()
    return new Set(
      state.results
        .filter(item => {
          if (seen.has(item.id)) return false
          seen.add(item.id)
          return true
        })
        .map(item => item.chunk_id)
    )
  }, [state.results])

  const isResultOnPage = useCallback(
    (resourceId: string) => state.results.some(item => item.id === resourceId),
    [state.results]
  )

  const showResult = useCallback((resourceId: string) => {
    document
      .getElementById(resultCardDomId(resourceId))
      ?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setHighlightedId(resourceId)
    clearTimeout(highlightTimer.current)
    highlightTimer.current = setTimeout(() => {
      setHighlightedId(null)
    }, CITATION_HIGHLIGHT_MS)
  }, [])

  useEffect(
    () => () => {
      clearTimeout(highlightTimer.current)
    },
    []
  )

  // Commit the query box. Changing the query always resets to page 1.
  const submitQuery = useCallback(() => {
    const trimmed = queryInput.trim()
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (trimmed) {
        params.set('q', trimmed)
      } else {
        params.delete('q')
      }
      params.set('page', '1')
      return params
    })
  }, [queryInput, setSearchParams])

  // Update a single filter param (type/project), resetting to page 1.
  const setFilterParam = useCallback(
    (key: string, value: string | undefined) => {
      setSearchParams(prev => {
        const params = new URLSearchParams(prev)
        if (value) {
          params.set(key, value)
        } else {
          params.delete(key)
        }
        params.set('page', '1')
        return params
      })
    },
    [setSearchParams]
  )

  const clearPreExpanded = useCallback(() => {
    setPreExpandedQuery(null)
  }, [])

  if (!currentTeam) return null

  return (
    <ListPage>
      <ListPage.Header
        title="Search"
        description="Find prompts, artifacts, blueprints, and memories across your team."
      />

      <ListPage.Container>
        <ListPage.Filters>
          <SearchFilters
            queryInput={queryInput}
            onQueryInputChange={setQueryInput}
            onSubmit={submitQuery}
            type={type}
            onTypeChange={value => {
              setFilterParam('type', value)
            }}
            projects={projects}
            selectedProjectId={projectId}
            onProjectChange={value => {
              setFilterParam('project', value)
            }}
          />
        </ListPage.Filters>

        {!hasQuery ? (
          <div className="p-4">
            <EmptyState
              icon={SearchIcon}
              title="Type to search"
              description="Enter a query above to find prompts, artifacts, blueprints, and memories."
            />
          </div>
        ) : (
          <>
            {state.status === 'ready' &&
              state.results.length > 0 &&
              aiSummary.key && (
                <div className="px-4 pt-4">
                  <AiSummary
                    availability={state.aiSummary}
                    canConfigure={can('team.update')}
                    teamId={currentTeam.id}
                    state={aiSummary.state}
                    onGenerate={aiSummary.generate}
                    onRetry={aiSummary.retry}
                    isOnPage={isResultOnPage}
                    onShowResult={showResult}
                    preExpanded={preExpandedQuery === q}
                    onToggle={clearPreExpanded}
                  />
                </div>
              )}
            <ListPage.Body
              status={state.status}
              errorTitle="Failed to search"
              errorMessage={state.error}
              empty={
                <EmptyState
                  icon={SearchIcon}
                  title="No matches found"
                  description={`Nothing matched “${q}”. Try a different search or adjust the filters.`}
                />
              }
            >
              <div className="space-y-3 p-4">
                {state.results.map(item => (
                  <SearchResultCard
                    key={item.chunk_id}
                    item={item}
                    domId={
                      firstCardIds.has(item.chunk_id)
                        ? resultCardDomId(item.id)
                        : undefined
                    }
                    highlighted={highlightedId === item.id}
                    expanded={expanded.has(item.chunk_id)}
                    onToggleExpand={() => {
                      toggleExpand(item.chunk_id)
                    }}
                  />
                ))}
              </div>
            </ListPage.Body>

            <ListPage.Footer
              count={
                state.status === 'loading' || state.status === 'error'
                  ? undefined
                  : {
                      visible: state.results.length,
                      total: state.total,
                      noun: 'result',
                    }
              }
              pagination={{
                page: state.page,
                totalPages: state.totalPages,
                onPageChange: nextPage => {
                  setSearchParams(prev => {
                    const params = new URLSearchParams(prev)
                    params.set('page', String(nextPage))
                    return params
                  })
                },
              }}
              hideCount={state.status === 'loading'}
            />
          </>
        )}
      </ListPage.Container>
    </ListPage>
  )
}
