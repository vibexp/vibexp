import type { ReactNode } from 'react'

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
import { DateTimeRangeFilter } from '@/pages/admin/filters/DateTimeRangeFilter'
import { NumberRangeFilter } from '@/pages/admin/filters/NumberRangeFilter'
import { AdminSavedFiltersMenu } from '@/pages/admin/savedFilters/AdminSavedFiltersMenu'
import type { PresetQuery } from '@/pages/admin/savedFilters/useAdminSavedFilters'
import {
  LAST_RESOURCE_CREATED,
  USER_RANGE_FILTERS,
} from '@/pages/admin/users/userAdvancedFilters'

/** `all` sends no `status` param; the other two map to the API enum. */
export type UserStatusFilter = 'all' | 'active' | 'suspended'

/**
 * Identity providers offered in the filter.
 *
 * A fixed list rather than a distinct-values query, because the API has no
 * endpoint for one. "Any provider" sends nothing, so an instance using a provider
 * not listed here is still fully reachable by name or email search — and the
 * create form takes the provider as free text for exactly that reason.
 */
export const IDP_PROVIDER_OPTIONS = ['google', 'github', 'oidc'] as const

export interface UserFiltersProps {
  searchInput: string
  onSearchInputChange: (value: string) => void
  status: UserStatusFilter
  onStatusChange: (value: UserStatusFilter) => void
  provider: string
  onProviderChange: (value: string) => void
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  onClear?: () => void
  hasActiveFilters: boolean
  /** Advanced panel (#1134): count ranges by base name, e.g. `prompt_count`. */
  getRange: (name: string) => NumberRangeValue
  onRangeChange: (name: string, value: NumberRangeValue) => void
  getDateRange: (name: string) => DateRangeValue
  onDateRangeChange: (name: string, value: DateRangeValue) => void
  advancedActiveCount: number
  /** Saved filter presets (#1148): the current filters as a preset query. */
  currentQuery: PresetQuery
  onApplyPreset: (query: PresetQuery) => void
  /** Actions on the filtered set (the CSV export, #1150). */
  actions?: ReactNode
}

export function UserFilters({
  searchInput,
  onSearchInputChange,
  status,
  onStatusChange,
  provider,
  onProviderChange,
  created,
  onCreatedChange,
  onClear,
  hasActiveFilters,
  getRange,
  onRangeChange,
  getDateRange,
  onDateRangeChange,
  advancedActiveCount,
  currentQuery,
  onApplyPreset,
  actions,
}: Readonly<UserFiltersProps>) {
  const advanced = (
    <>
      {USER_RANGE_FILTERS.map(filter => (
        <NumberRangeFilter
          key={filter.name}
          label={filter.label}
          value={getRange(filter.name)}
          onChange={value => {
            onRangeChange(filter.name, value)
          }}
        />
      ))}
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
      searchPlaceholder="Search email or name…"
      searchLabel="Search users"
      created={created}
      onCreatedChange={onCreatedChange}
      onClear={onClear}
      hasActiveFilters={hasActiveFilters}
      advanced={advanced}
      advancedActiveCount={advancedActiveCount}
      actions={actions}
      presets={
        <AdminSavedFiltersMenu
          list="users"
          currentQuery={currentQuery}
          onApply={onApplyPreset}
        />
      }
    >
      <Select
        value={status}
        onValueChange={value => {
          onStatusChange(value as UserStatusFilter)
        }}
      >
        <SelectTrigger className="w-control-sm" aria-label="Account status">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All statuses</SelectItem>
          <SelectItem value="active">Active</SelectItem>
          <SelectItem value="suspended">Suspended</SelectItem>
        </SelectContent>
      </Select>

      <Select
        value={provider || 'all'}
        onValueChange={value => {
          onProviderChange(value === 'all' ? '' : value)
        }}
      >
        <SelectTrigger className="w-[160px]" aria-label="Identity provider">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">Any provider</SelectItem>
          {IDP_PROVIDER_OPTIONS.map(option => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </AdminFilterBar>
  )
}
