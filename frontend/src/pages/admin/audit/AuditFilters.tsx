import type { ReactNode } from 'react'
import { useCallback } from 'react'

import type { DateRangeValue } from '@/components/ui/date-range'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'
import { AdminSearchPicker } from '@/pages/admin/filters/AdminSearchPicker'
import { GroupHeading } from '@/pages/admin/filters/GroupHeading'
import { useAdminPagedSearch } from '@/pages/admin/filters/useAdminPagedSearch'
import { AdminSavedFiltersMenu } from '@/pages/admin/savedFilters/AdminSavedFiltersMenu'
import type { PresetQuery } from '@/pages/admin/savedFilters/useAdminSavedFilters'
import { AdminTeamPicker } from '@/pages/admin/teams/AdminTeamPicker'
import type {
  AdminProjectListItem,
  AdminUserListItem,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AuditChoiceFilter } from './AuditChoiceFilter'
import {
  AUDIT_OUTCOMES,
  AUDIT_RESOURCE_TYPES,
  AUDIT_SOURCES,
} from './auditListParams'
import { outcomeLabel, sourceLabel } from './auditPresentation'
import {
  AUDIT_OPERATION_GROUPS,
  AUDIT_OPERATIONS,
  type AuditOperation,
} from './operationLabels'

const OPERATION_CHOICES = (
  Object.keys(AUDIT_OPERATIONS) as AuditOperation[]
).map(value => ({
  value,
  label: AUDIT_OPERATIONS[value].label,
  group: AUDIT_OPERATIONS[value].group,
}))
const SOURCE_CHOICES = AUDIT_SOURCES.map(value => ({
  value,
  label: sourceLabel(value),
}))
const OUTCOME_CHOICES = AUDIT_OUTCOMES.map(value => ({
  value,
  label: outcomeLabel(value),
}))

/**
 * At phone width every picker takes the full row and the operation filter and
 * date range share one row in equal halves, so the bar has no ragged gutter.
 * From sm up the date trigger is as wide as its label (at most 180px) and Clear
 * is a short "Clear", which keep the actions on the second row at 1280 with a
 * filter applied (#1551).
 */
const PICKER_WIDTH = 'w-full sm:w-[200px]'
const HALF_WIDTH = 'w-[calc(50%-0.25rem)] sm:w-[180px]'
const DATE_WIDTH = 'w-[calc(50%-0.25rem)] sm:w-auto sm:max-w-[180px]'

export interface AuditFiltersProps {
  /** Uncommitted free text; the page debounces it into the URL as `q`. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  /** The range in effect: the URL's, or the default last seven days. */
  range: DateRangeValue
  onRangeChange: (value: DateRangeValue) => void
  actorUserId: string
  onActorUserIdChange: (id: string) => void
  teamId: string
  onTeamIdChange: (id: string) => void
  projectId: string
  onProjectIdChange: (id: string) => void
  operations: readonly string[]
  onOperationsChange: (value: string[]) => void
  resourceType: string
  onResourceTypeChange: (value: string) => void
  sources: readonly string[]
  onSourcesChange: (value: string[]) => void
  outcomes: readonly string[]
  onOutcomesChange: (value: string[]) => void
  onClear: () => void
  hasActiveFilters: boolean
  currentQuery: PresetQuery
  onApplyPreset: (query: PresetQuery) => void
  /** The CSV export of the filtered log. */
  actions?: ReactNode
}

const userId = (user: AdminUserListItem) => user.id
const userLabel = (user: AdminUserListItem) => user.name || user.email
const userDetail = (user: AdminUserListItem) =>
  user.name ? user.email : undefined
const resolveUserLabel = async (id: string) => {
  const user = await adminService.getUser(id)
  return user.name || user.email
}

const projectId = (project: AdminProjectListItem) => project.id
const projectLabel = (project: AdminProjectListItem) => project.name
const projectDetail = (project: AdminProjectListItem) => project.team.name
const resolveProjectLabel = async (id: string) =>
  (await adminService.getProject(id)).name

/**
 * The audit log's filter bar (#1517): free text, the date range, pickers for
 * the user, team and project (searched by name or email; the URL keeps the id),
 * the grouped operation filter, and — under Advanced — resource type, channel
 * and outcome.
 */
export function AuditFilters(props: Readonly<AuditFiltersProps>) {
  const fetchUsers = useCallback(
    async (search: string | undefined, page: number, limit: number) => {
      const response = await adminService.listUsers({
        page,
        limit,
        search,
        sort_by: 'email',
        sort_order: 'asc',
      })
      return { items: response.users, totalPages: response.total_pages }
    },
    []
  )
  const fetchProjects = useCallback(
    async (search: string | undefined, page: number, limit: number) => {
      const response = await adminService.listProjects({
        page,
        limit,
        search,
        sort_by: 'name',
        sort_order: 'asc',
      })
      return { items: response.projects, totalPages: response.total_pages }
    },
    []
  )
  const userSearch = useAdminPagedSearch(fetchUsers, 'Failed to load users')
  const projectSearch = useAdminPagedSearch(
    fetchProjects,
    'Failed to load projects'
  )

  const advancedActiveCount =
    (props.resourceType ? 1 : 0) +
    (props.sources.length > 0 ? 1 : 0) +
    (props.outcomes.length > 0 ? 1 : 0)

  const advanced = (
    <>
      <div className="space-y-1.5">
        <GroupHeading>Resource type</GroupHeading>
        <AuditChoiceFilter
          value={props.resourceType ? [props.resourceType] : []}
          onChange={value => {
            props.onResourceTypeChange(value[0] ?? '')
          }}
          options={AUDIT_RESOURCE_TYPES}
          multiple={false}
          label="Filter by resource type"
          allLabel="All resource types"
          noun="resource types"
          className="w-full"
        />
      </div>
      <div className="space-y-1.5">
        <GroupHeading>Channel</GroupHeading>
        <AuditChoiceFilter
          value={props.sources}
          onChange={props.onSourcesChange}
          options={SOURCE_CHOICES}
          label="Filter by channel"
          allLabel="All channels"
          noun="channels"
          className="w-full"
        />
      </div>
      <div className="space-y-1.5">
        <GroupHeading>Outcome</GroupHeading>
        <AuditChoiceFilter
          value={props.outcomes}
          onChange={props.onOutcomesChange}
          options={OUTCOME_CHOICES}
          label="Filter by outcome"
          allLabel="All outcomes"
          noun="outcomes"
          className="w-full"
        />
      </div>
    </>
  )

  return (
    <AdminFilterBar
      searchInput={props.searchInput}
      onSearchInputChange={props.onSearchInputChange}
      searchPlaceholder="Search resource, email or IP…"
      searchLabel="Search the audit log"
      created={props.range}
      onCreatedChange={props.onRangeChange}
      createdLabel="Filter by date"
      createdClassName={DATE_WIDTH}
      compactClear
      onClear={props.onClear}
      hasActiveFilters={props.hasActiveFilters}
      advanced={advanced}
      advancedActiveCount={advancedActiveCount}
      actions={props.actions}
      presets={
        <AdminSavedFiltersMenu
          list="audit"
          currentQuery={props.currentQuery}
          onApply={props.onApplyPreset}
        />
      }
    >
      <AdminSearchPicker
        value={props.actorUserId}
        onChange={props.onActorUserIdChange}
        search={userSearch}
        getId={userId}
        getLabel={userLabel}
        getDetail={userDetail}
        resolveLabel={resolveUserLabel}
        allLabel="All users"
        selectedFallback="Selected user"
        searchPlaceholder="Search name or email…"
        emptyText="No users found."
        aria-label="Filter by user"
        className={PICKER_WIDTH}
      />
      <AdminTeamPicker
        value={props.teamId}
        onChange={props.onTeamIdChange}
        resolveName
        className={PICKER_WIDTH}
      />
      <AdminSearchPicker
        value={props.projectId}
        onChange={props.onProjectIdChange}
        search={projectSearch}
        getId={projectId}
        getLabel={projectLabel}
        getDetail={projectDetail}
        resolveLabel={resolveProjectLabel}
        allLabel="All projects"
        selectedFallback="Selected project"
        searchPlaceholder="Search projects…"
        emptyText="No projects found."
        aria-label="Filter by project"
        className={PICKER_WIDTH}
      />
      <AuditChoiceFilter
        value={props.operations}
        onChange={props.onOperationsChange}
        options={OPERATION_CHOICES}
        groups={AUDIT_OPERATION_GROUPS}
        label="Filter by operation"
        allLabel="All operations"
        noun="operations"
        searchable
        className={HALF_WIDTH}
      />
    </AdminFilterBar>
  )
}
