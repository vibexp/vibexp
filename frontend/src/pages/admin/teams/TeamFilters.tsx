import type { DateRangeValue } from '@/components/ui/date-range'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'
import type { NumberRangeValue } from '@/pages/admin/filters/advancedFilterParams'
import { GroupHeading } from '@/pages/admin/filters/GroupHeading'
import { NumberRangeFilter } from '@/pages/admin/filters/NumberRangeFilter'
import { OwnerEmailFilter } from '@/pages/admin/filters/OwnerEmailFilter'
import { TriStateFilter } from '@/pages/admin/filters/TriStateFilter'
import type { TeamRangeFilter } from '@/pages/admin/teams/teamListParams'
import {
  TEAM_MEMBERSHIP_RANGES,
  TEAM_RESOURCE_RANGES,
  TEAM_SETUP_TRISTATES,
} from '@/pages/admin/teams/teamListParams'

/**
 * The three states of the personal/shared filter.
 *
 * A tri-state select rather than a checkbox, because `is_personal` is an
 * **optional** boolean on the API: "All" must send no parameter at all, and
 * `is_personal=false` means "shared only" — a checkbox cannot express the
 * difference, and conflating them would silently hide every personal workspace.
 */
export type TeamKindFilter = 'all' | 'shared' | 'personal'

export interface TeamFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  kind: TeamKindFilter
  onKindChange: (value: TeamKindFilter) => void
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  /** Shown only while at least one filter is applied. */
  onClear?: () => void
  hasActiveFilters: boolean
  /** Advanced panel (#1139): count ranges by base name, e.g. `owner_count`. */
  getRange: (name: string) => NumberRangeValue
  onRangeChange: (name: string, value: NumberRangeValue) => void
  getTriState: (name: string) => boolean | undefined
  onTriStateChange: (name: string, value: boolean | undefined) => void
  ownerEmail: string
  onOwnerEmailChange: (value: string) => void
  /**
   * Bumped on every Clear. Remounts the owner-email input, because a rejected
   * draft never reached the URL, so the URL alone cannot tell it to reset.
   */
  ownerEmailResetKey: number
  advancedActiveCount: number
}

export function TeamFilters({
  searchInput,
  onSearchInputChange,
  kind,
  onKindChange,
  created,
  onCreatedChange,
  onClear,
  hasActiveFilters,
  getRange,
  onRangeChange,
  getTriState,
  onTriStateChange,
  ownerEmail,
  onOwnerEmailChange,
  ownerEmailResetKey,
  advancedActiveCount,
}: Readonly<TeamFiltersProps>) {
  const range = (filter: TeamRangeFilter) => (
    <NumberRangeFilter
      key={filter.name}
      label={filter.label}
      value={getRange(filter.name)}
      onChange={value => {
        onRangeChange(filter.name, value)
      }}
    />
  )

  const advanced = (
    <>
      <GroupHeading>Membership</GroupHeading>
      {TEAM_MEMBERSHIP_RANGES.map(range)}
      {/* The API matches the team's primary owner (`teams.owner_id`) only. */}
      <OwnerEmailFilter
        key={ownerEmailResetKey}
        label="Primary owner email"
        value={ownerEmail}
        onChange={onOwnerEmailChange}
      />

      <GroupHeading>Resources</GroupHeading>
      {TEAM_RESOURCE_RANGES.map(range)}

      <GroupHeading>Setup</GroupHeading>
      {TEAM_SETUP_TRISTATES.map(filter => (
        <TriStateFilter
          key={filter.name}
          label={filter.label}
          value={getTriState(filter.name)}
          onChange={value => {
            onTriStateChange(filter.name, value)
          }}
        />
      ))}
    </>
  )

  return (
    <AdminFilterBar
      searchInput={searchInput}
      onSearchInputChange={onSearchInputChange}
      searchPlaceholder="Search name, slug, or owner email…"
      searchLabel="Search teams"
      created={created}
      onCreatedChange={onCreatedChange}
      onClear={onClear}
      hasActiveFilters={hasActiveFilters}
      advanced={advanced}
      advancedActiveCount={advancedActiveCount}
    >
      <Select
        value={kind}
        onValueChange={value => {
          onKindChange(value as TeamKindFilter)
        }}
      >
        <SelectTrigger className="w-[160px]" aria-label="Team type">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All teams</SelectItem>
          <SelectItem value="shared">Shared only</SelectItem>
          <SelectItem value="personal">Personal only</SelectItem>
        </SelectContent>
      </Select>
    </AdminFilterBar>
  )
}
