import type { DateRangeValue } from '@/components/ui/date-range'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'
import type { NumberRangeValue } from '@/pages/admin/filters/advancedFilterParams'
import { DateTimeRangeFilter } from '@/pages/admin/filters/DateTimeRangeFilter'
import { GroupHeading } from '@/pages/admin/filters/GroupHeading'
import { NumberRangeFilter } from '@/pages/admin/filters/NumberRangeFilter'
import { OwnerEmailFilter } from '@/pages/admin/filters/OwnerEmailFilter'
import {
  LAST_RESOURCE_CREATED,
  PROJECT_RESOURCE_RANGES,
} from '@/pages/admin/projects/projectListParams'
import { AdminSavedFiltersMenu } from '@/pages/admin/savedFilters/AdminSavedFiltersMenu'
import type { PresetQuery } from '@/pages/admin/savedFilters/useAdminSavedFilters'
import { AdminTeamPicker } from '@/pages/admin/teams/AdminTeamPicker'

export interface ProjectFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  teamId: string
  onTeamIdChange: (teamId: string) => void
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  onClear?: () => void
  hasActiveFilters: boolean
  /** Advanced panel (#1144): count ranges by base name, e.g. `prompt_count`. */
  getRange: (name: string) => NumberRangeValue
  onRangeChange: (name: string, value: NumberRangeValue) => void
  getDateRange: (name: string) => DateRangeValue
  onDateRangeChange: (name: string, value: DateRangeValue) => void
  ownerEmail: string
  onOwnerEmailChange: (value: string) => void
  /**
   * Bumped on every Clear. Remounts the creator-email input, because a rejected
   * draft never reached the URL, so the URL alone cannot tell it to reset.
   */
  ownerEmailResetKey: number
  advancedActiveCount: number
  /** Saved filter presets (#1148): the current filters as a preset query. */
  currentQuery: PresetQuery
  onApplyPreset: (query: PresetQuery) => void
}

export function ProjectFilters({
  searchInput,
  onSearchInputChange,
  teamId,
  onTeamIdChange,
  created,
  onCreatedChange,
  onClear,
  hasActiveFilters,
  getRange,
  onRangeChange,
  getDateRange,
  onDateRangeChange,
  ownerEmail,
  onOwnerEmailChange,
  ownerEmailResetKey,
  advancedActiveCount,
  currentQuery,
  onApplyPreset,
}: Readonly<ProjectFiltersProps>) {
  const advanced = (
    <>
      <GroupHeading>Owner</GroupHeading>
      {/* The API matches the project's creator (`projects.user_id`, the Owner
          column), not the owning team's owner. */}
      <OwnerEmailFilter
        key={ownerEmailResetKey}
        label="Creator email"
        value={ownerEmail}
        onChange={onOwnerEmailChange}
      />

      <GroupHeading>Resources</GroupHeading>
      {PROJECT_RESOURCE_RANGES.map(filter => (
        <NumberRangeFilter
          key={filter.name}
          label={filter.label}
          value={getRange(filter.name)}
          onChange={value => {
            onRangeChange(filter.name, value)
          }}
        />
      ))}

      <GroupHeading>Activity</GroupHeading>
      <DateTimeRangeFilter
        label="Last resource created"
        value={getDateRange(LAST_RESOURCE_CREATED)}
        onChange={value => {
          onDateRangeChange(LAST_RESOURCE_CREATED, value)
        }}
      />
    </>
  )

  return (
    <AdminFilterBar
      searchInput={searchInput}
      onSearchInputChange={onSearchInputChange}
      searchPlaceholder="Search name or slug…"
      searchLabel="Search projects"
      created={created}
      onCreatedChange={onCreatedChange}
      onClear={onClear}
      hasActiveFilters={hasActiveFilters}
      advanced={advanced}
      advancedActiveCount={advancedActiveCount}
      presets={
        <AdminSavedFiltersMenu
          list="projects"
          currentQuery={currentQuery}
          onApply={onApplyPreset}
        />
      }
    >
      <AdminTeamPicker value={teamId} onChange={onTeamIdChange} />
    </AdminFilterBar>
  )
}
