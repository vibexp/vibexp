import { useId, useRef, useState } from 'react'

import type { DateRangeValue } from '@/components/ui/date-range'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'
import type { NumberRangeValue } from '@/pages/admin/filters/advancedFilterParams'
import { NumberRangeFilter } from '@/pages/admin/filters/NumberRangeFilter'
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
  advancedActiveCount: number
}

function GroupHeading({ children }: Readonly<{ children: string }>) {
  return (
    <h3 className="text-muted-foreground col-span-full text-xs font-semibold uppercase tracking-wide">
      {children}
    </h3>
  )
}

/**
 * Owner email, committed on blur or Enter like the range inputs, so typing an
 * address does not fire a request per keystroke.
 *
 * The API matches the team's primary owner (`teams.owner_id`) exactly, and
 * rejects a malformed address with a 400 — so a half-typed address is marked
 * invalid and never committed, rather than replacing the list with an error that
 * a reload would repeat.
 */
function OwnerEmailFilter({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (value: string) => void }>) {
  const id = useId()
  const errorId = `${id}-error`
  const inputRef = useRef<HTMLInputElement>(null)
  const [draft, setDraft] = useState(value)
  const [invalid, setInvalid] = useState(false)

  // Follow the committed value when it changes from outside (URL restore,
  // Clear), adjusting state during render rather than in an effect.
  const [committed, setCommitted] = useState(value)
  if (committed !== value) {
    setCommitted(value)
    setDraft(value)
    setInvalid(false)
  }

  const commit = () => {
    const next = draft.trim()
    if (next !== '' && inputRef.current?.validity.typeMismatch === true) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    if (next !== value) onChange(next)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium leading-none">
        Primary owner email
      </label>
      <Input
        ref={inputRef}
        id={id}
        type="email"
        placeholder="owner@example.com"
        value={draft}
        onChange={event => {
          setDraft(event.target.value)
        }}
        aria-invalid={invalid || undefined}
        aria-describedby={invalid ? errorId : undefined}
        onBlur={commit}
        onKeyDown={event => {
          if (event.key === 'Enter') commit()
        }}
      />
      {invalid && (
        <p id={errorId} role="alert" className="text-destructive text-xs">
          Enter a full email address
        </p>
      )}
    </div>
  )
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
      <OwnerEmailFilter value={ownerEmail} onChange={onOwnerEmailChange} />

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
