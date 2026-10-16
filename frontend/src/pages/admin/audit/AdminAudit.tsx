import type { ColumnDef } from '@tanstack/react-table'
import { ChevronLeft, ChevronRight, History } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import type { DateRangeValue } from '@/components/ui/date-range'
import { AdminExportButton } from '@/pages/admin/AdminExportButton'
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { AdminAuditEvent } from '@/services/adminAuditService'
import { adminAuditService } from '@/services/adminAuditService'
import { getErrorMessage } from '@/utils/errorHandling'

import { AuditEventDrawer } from './AuditEventDrawer'
import { AuditFilters } from './AuditFilters'
import {
  AUDIT_FILTER_DEFAULTS,
  AUDIT_FILTER_KEYS,
  AUDIT_PAGE_SIZE,
  auditExportParams,
  buildAuditListParams,
  effectiveAuditRange,
  joinList,
  selectedOperations,
  selectedOutcomes,
  selectedSources,
} from './auditListParams'
import {
  AuditActor,
  AuditOutcomeBadge,
  AuditResource,
  auditScope,
  AuditSourceBadge,
  AuditTime,
} from './auditPresentation'
import { operationLabel } from './operationLabels'
import { useAuditCursor } from './useAuditCursor'

/** The list has one fixed order, newest first; nothing is sortable. */
const SORTABLE_KEYS = ['created_at'] as const

interface State {
  events: AdminAuditEvent[]
  nextCursor: string | null
  loading: boolean
  error: string | null
}

const INITIAL: State = {
  events: [],
  nextCursor: null,
  loading: true,
  error: null,
}

/**
 * The instance audit log (#1517): who did what, when, through which channel,
 * and how it ended — filterable, newest first, paged by cursor, with a detail
 * drawer, a CSV export of the filter and saved filter presets.
 *
 * Gated by the admin shell like every admin page; the API 404s non-admins
 * regardless.
 */
export function AdminAudit() {
  const {
    filters,
    setFilters,
    searchInput,
    setSearchInput,
    created,
    setCreated,
    hasActiveFilters,
    handleClear,
    currentQuery,
    applyQuery,
  } = useAdminListFilters({
    defaults: AUDIT_FILTER_DEFAULTS,
    sortableKeys: SORTABLE_KEYS,
    defaultSort: 'created_at',
    filterKeys: AUDIT_FILTER_KEYS,
  })

  // Pinned for the page's lifetime, so the default range does not drift (and
  // refetch) as the clock moves while the page is open.
  const [now] = useState(() => new Date())
  const { range } = useMemo(
    () => effectiveAuditRange(created, now),
    [created, now]
  )

  const baseParams = useMemo(
    () => buildAuditListParams(filters, { range, limit: AUDIT_PAGE_SIZE }),
    [filters, range]
  )
  const filterKey = JSON.stringify(baseParams)
  const cursor = useAuditCursor(filterKey)
  const params = useMemo(
    () =>
      cursor.cursor === undefined
        ? baseParams
        : { ...baseParams, cursor: cursor.cursor },
    [baseParams, cursor.cursor]
  )

  const [state, setState] = useState<State>(INITIAL)
  const [selected, setSelected] = useState<AdminAuditEvent | null>(null)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminAuditService
      .listEvents(params)
      .then(page => {
        if (cancelled) return
        setState({
          events: page.items,
          nextCursor: page.next_cursor,
          loading: false,
          error: null,
        })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(err, 'Failed to load the audit log'),
        }))
      })
    return () => {
      // Guards the debounced-search race: a slow response for an earlier filter
      // must not overwrite the results of a newer one.
      cancelled = true
    }
  }, [params])

  const setList = useCallback(
    (key: string) => (value: string[]) => {
      setFilters({ [key]: joinList(value) })
    },
    [setFilters]
  )
  const setOne = useCallback(
    (key: string) => (value: string) => {
      setFilters({ [key]: value })
    },
    [setFilters]
  )
  const handleRangeChange = useCallback(
    (value: DateRangeValue) => {
      setCreated(value)
    },
    [setCreated]
  )

  const columns = useMemo<ColumnDef<AdminAuditEvent>[]>(
    () => [
      {
        accessorKey: 'created_at',
        header: 'Time',
        cell: ({ row }) => <AuditTime at={row.original.created_at} />,
      },
      {
        id: 'actor',
        header: 'Actor',
        cell: ({ row }) => <AuditActor event={row.original} />,
      },
      {
        id: 'operation',
        header: 'Operation',
        cell: ({ row }) => (
          <span className="text-sm">
            {operationLabel(row.original.operation)}
          </span>
        ),
      },
      {
        id: 'resource',
        header: 'Resource',
        cell: ({ row }) => <AuditResource event={row.original} />,
      },
      {
        id: 'scope',
        header: 'Team / project',
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {auditScope(row.original)}
          </span>
        ),
      },
      {
        id: 'source',
        header: 'Channel',
        cell: ({ row }) => <AuditSourceBadge event={row.original} />,
      },
      {
        id: 'outcome',
        header: 'Outcome',
        cell: ({ row }) => <AuditOutcomeBadge outcome={row.original.outcome} />,
      },
    ],
    []
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.events.length === 0
  )

  return (
    <ListPage>
      <ListPage.Container>
        <ListPage.Filters>
          <AuditFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            range={range}
            onRangeChange={handleRangeChange}
            actorUserId={filters.actor_user_id}
            onActorUserIdChange={setOne('actor_user_id')}
            teamId={filters.team_id}
            onTeamIdChange={setOne('team_id')}
            projectId={filters.project_id}
            onProjectIdChange={setOne('project_id')}
            operations={selectedOperations(filters.operation)}
            onOperationsChange={setList('operation')}
            resourceType={filters.resource_type}
            onResourceTypeChange={setOne('resource_type')}
            sources={selectedSources(filters.source)}
            onSourcesChange={setList('source')}
            outcomes={selectedOutcomes(filters.outcome)}
            onOutcomesChange={setList('outcome')}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
            currentQuery={currentQuery}
            onApplyPreset={applyQuery}
            actions={
              <AdminExportButton
                noun="audit events"
                disabled={status === 'empty'}
                onExport={() =>
                  adminAuditService.exportEvents(auditExportParams(baseParams))
                }
              />
            }
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load the audit log"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={History}
              title="No audit events match these filters"
              description="Try a wider date range or fewer filters."
              actions={
                hasActiveFilters ? (
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                ) : undefined
              }
            />
          }
        >
          <div className="hidden md:block">
            <ListTable
              rows={state.events}
              columns={columns}
              onRowClick={setSelected}
            />
          </div>
          <AuditEventCards events={state.events} onSelect={setSelected} />
        </ListPage.Body>

        <AuditPager
          pageNumber={cursor.pageNumber}
          hasPrevious={cursor.hasPrevious}
          nextCursor={state.loading ? null : state.nextCursor}
          onPrevious={cursor.previous}
          onNext={cursor.next}
          hidden={status === 'loading' && !cursor.hasPrevious}
        />
      </ListPage.Container>

      <AuditEventDrawer
        event={selected}
        onClose={() => {
          setSelected(null)
        }}
      />
    </ListPage>
  )
}

/**
 * The phone-width rendering (#1517): one card per event, stacked, so nothing
 * scrolls sideways. Each card is a button opening the drawer.
 */
function AuditEventCards({
  events,
  onSelect,
}: Readonly<{
  events: AdminAuditEvent[]
  onSelect: (event: AdminAuditEvent) => void
}>) {
  return (
    <ul className="divide-y md:hidden" aria-label="Audit events">
      {events.map(event => (
        <li key={event.id}>
          <button
            type="button"
            className="hover:bg-muted/40 flex w-full flex-col gap-2 p-3 text-left"
            onClick={() => {
              onSelect(event)
            }}
          >
            <span className="flex items-start justify-between gap-2">
              <span className="text-sm font-medium">
                {operationLabel(event.operation)}
              </span>
              <AuditOutcomeBadge outcome={event.outcome} />
            </span>
            <AuditActor event={event} />
            {event.resource_type && <AuditResource event={event} />}
            <span className="flex flex-wrap items-center gap-2">
              <AuditSourceBadge event={event} />
              {(event.team_name ?? event.project_name) && (
                <span className="text-muted-foreground min-w-0 truncate text-xs">
                  {auditScope(event)}
                </span>
              )}
              <span className="ml-auto">
                <AuditTime at={event.created_at} />
              </span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

/**
 * Previous / Next over the keyset cursor. There is no total count, by design
 * (#1516), so the footer names the page rather than "x of y".
 */
function AuditPager({
  pageNumber,
  hasPrevious,
  nextCursor,
  onPrevious,
  onNext,
  hidden,
}: Readonly<{
  pageNumber: number
  hasPrevious: boolean
  nextCursor: string | null
  onPrevious: () => void
  onNext: (cursor: string) => void
  hidden: boolean
}>) {
  if (hidden || (!hasPrevious && nextCursor === null)) return null
  return (
    <nav
      className="flex items-center justify-end gap-2"
      aria-label="Audit log pages"
    >
      <span className="text-muted-foreground mr-auto text-xs">
        Page {pageNumber}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={!hasPrevious}
        onClick={onPrevious}
      >
        <ChevronLeft className="size-4" aria-hidden />
        Previous
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={nextCursor === null}
        onClick={() => {
          if (nextCursor !== null) onNext(nextCursor)
        }}
      >
        Next
        <ChevronRight className="size-4" aria-hidden />
      </Button>
    </nav>
  )
}
