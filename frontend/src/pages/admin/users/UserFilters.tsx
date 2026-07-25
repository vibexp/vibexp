import type { DateRangeValue } from '@/components/ui/date-range'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'

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
}: Readonly<UserFiltersProps>) {
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
    >
      <Select
        value={status}
        onValueChange={value => {
          onStatusChange(value as UserStatusFilter)
        }}
      >
        <SelectTrigger className="w-[150px]" aria-label="Account status">
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
