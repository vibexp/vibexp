import { Search } from 'lucide-react'
import type { ReactNode } from 'react'

import { FreshnessFilterSelect } from '@/components/FreshnessFilterSelect'
import { MetadataFilterField } from '@/components/metadata/MetadataFilterField'
import type {
  FilterSpec,
  ResourceDescriptor,
} from '@/components/patterns/resource'
import { TaxonomyFilter } from '@/components/TaxonomyFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { usePromptLabels } from '@/hooks/usePromptLabels'
import { useTypes } from '@/hooks/useTypes'
import type {
  MetadataFilterValue,
  MetadataResourceType,
} from '@/services/metadataService'

import { FILTER_ALL, FILTER_CONTROL_WIDTH } from './filterControls'

/**
 * A resource list's filter bar, generated from `descriptor.list.filters`.
 *
 * `ArtifactFilters`, `BlueprintFilters` and `MemoryFilters` were near-verbatim
 * copies of one another, and the drift that produced was not cosmetic:
 * blueprints had a status the API filters on and no control for it, prompts had
 * labels and no control for it (#908). Which controls a list offers is now a
 * property of the resource type, so adding one is a descriptor edit.
 *
 * ## What this does NOT own
 *
 * The filter *state* — URL sync, the debounced search, the metadata round-trip,
 * `hasActiveFilters`, `handleClear` — stays in `useResourceListFilters`. This is
 * only the presentation layer above it, which is why the page still owns its
 * `FILTER_DEFAULTS` (`useUrlFilters` captures them once on mount, so they must
 * be a module-level constant per page).
 *
 * ## The value contract
 *
 * `values` holds the raw URL strings, and `onChange` emits raw URL strings:
 * {@link FILTER_ALL} for a cleared `select`/`freshness`, `''` for a cleared
 * `taxonomy`. Coercing a value to what the API accepts stays with the page,
 * which is where the enum allowlists live — a junk `?status=bogus` must be
 * dropped from the request rather than forwarded into a 400.
 */
export interface ResourceFilterBarProps {
  descriptor: ResourceDescriptor
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  /** Committed filter values, keyed by filter key. */
  values: Readonly<Record<string, string>>
  onChange: (key: string, value: string) => void
  /** Required when the descriptor declares a `metadata` filter. */
  metadata?: MetadataFilterValue
  onMetadataChange?: (value: MetadataFilterValue) => void
  /** Narrows the metadata catalog to the globally selected project. */
  projectId?: string
  /**
   * Controls that are not fields of the resource — the prompt "Shared"
   * tri-state, which describes an active share rather than the prompt.
   */
  extras?: ReactNode
  /** Shown only while at least one filter is applied. */
  onClear?: () => void
  hasActiveFilters: boolean
}

interface SelectOption {
  value: string
  label: string
}

/** Options a `select` reads off its own `FieldSpec`, in declaration order. */
function fieldOptions(
  descriptor: ResourceDescriptor,
  key: string
): SelectOption[] {
  const field = descriptor.fields.find(candidate => candidate.key === key)
  const labels = new Map(Object.entries(field?.valueLabels ?? {}))
  const values = field?.statusValues ?? [...labels.keys()]
  return values.map(value => ({ value, label: labels.get(value) ?? value }))
}

interface FilterControlProps {
  spec: FilterSpec
  value: string
  onChange: (value: string) => void
}

/** The shared shape of every option Select on the bar. */
function SelectFilter({
  spec,
  value,
  onChange,
  options,
}: Readonly<FilterControlProps & { options: SelectOption[] }>) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className={FILTER_CONTROL_WIDTH}
        aria-label={spec.label}
        data-testid={spec.testId}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={FILTER_ALL}>{spec.allLabel ?? 'All'}</SelectItem>
        {options.map(option => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/**
 * Artifact types are the team's own registered types, so the option list is a
 * runtime catalog. Its own component so the fetch happens only on a list whose
 * descriptor declares the filter, instead of on every list that renders a bar.
 */
function TypeCatalogFilter({
  spec,
  value,
  onChange,
  resourceType,
}: Readonly<FilterControlProps & { resourceType: string }>) {
  const { types } = useTypes(resourceType)
  return (
    <SelectFilter
      spec={spec}
      value={value}
      onChange={onChange}
      options={types.map(type => ({ value: type.slug, label: type.name }))}
    />
  )
}

/** Same reasoning as `TypeCatalogFilter`, for the team's prompt labels. */
function LabelsCatalogFilter({
  spec,
  value,
  onChange,
}: Readonly<FilterControlProps>) {
  const { labels, loading, error, load } = usePromptLabels()
  return (
    <TaxonomyFilter
      value={value === '' ? [] : value.split(',')}
      onChange={next => {
        onChange(next.join(','))
      }}
      options={labels}
      loading={loading}
      error={error}
      onOpen={load}
      label={spec.label}
      testId={spec.testId}
    />
  )
}

function SearchFilter({
  spec,
  plural,
  searchInput,
  onSearchInputChange,
}: Readonly<{
  spec: FilterSpec
  plural: string
  searchInput: string
  onSearchInputChange: (value: string) => void
}>) {
  return (
    <div className="relative min-w-[240px] max-w-[480px] flex-1">
      <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
      <Input
        value={searchInput}
        onChange={e => {
          onSearchInputChange(e.target.value)
        }}
        placeholder={`Search ${plural}…`}
        aria-label={spec.label}
        className="pl-8"
      />
    </div>
  )
}

interface ResourceFilterControlProps extends FilterControlProps {
  descriptor: ResourceDescriptor
  searchInput: string
  onSearchInputChange: (value: string) => void
  metadata: MetadataFilterValue | undefined
  onMetadataChange: ((value: MetadataFilterValue) => void) | undefined
  projectId: string | undefined
}

/**
 * One control, chosen by `spec.control`. Split out of the bar so neither
 * function carries the whole dispatch — Sonar's cognitive-complexity limit is
 * 15 and a single switch over five controls plus their option sources is well
 * past it.
 */
function ResourceFilterControl({
  spec,
  value,
  onChange,
  descriptor,
  searchInput,
  onSearchInputChange,
  metadata,
  onMetadataChange,
  projectId,
}: Readonly<ResourceFilterControlProps>) {
  if (spec.control === 'search') {
    return (
      <SearchFilter
        spec={spec}
        plural={descriptor.plural}
        searchInput={searchInput}
        onSearchInputChange={onSearchInputChange}
      />
    )
  }

  if (spec.control === 'freshness') {
    return (
      <FreshnessFilterSelect
        value={value === 'stale' ? 'stale' : undefined}
        onChange={next => {
          onChange(next ?? FILTER_ALL)
        }}
        ariaLabel={spec.label}
        testId={spec.testId}
      />
    )
  }

  if (spec.control === 'metadata') {
    // A descriptor may declare the filter while a particular page has no
    // metadata state to give it; render nothing rather than a dead control.
    if (!metadata || !onMetadataChange) return null
    return (
      <MetadataFilterField
        resourceType={descriptor.plural as MetadataResourceType}
        projectId={projectId}
        value={metadata}
        onChange={onMetadataChange}
        ariaLabel={spec.label}
      />
    )
  }

  if (spec.control === 'taxonomy') {
    return <LabelsCatalogFilter spec={spec} value={value} onChange={onChange} />
  }

  if (spec.optionsFrom === 'types') {
    return (
      <TypeCatalogFilter
        spec={spec}
        value={value}
        onChange={onChange}
        resourceType={descriptor.plural}
      />
    )
  }

  return (
    <SelectFilter
      spec={spec}
      value={value}
      onChange={onChange}
      options={fieldOptions(descriptor, spec.key)}
    />
  )
}

export function ResourceFilterBar({
  descriptor,
  searchInput,
  onSearchInputChange,
  values,
  onChange,
  metadata,
  onMetadataChange,
  projectId,
  extras,
  onClear,
  hasActiveFilters,
}: Readonly<ResourceFilterBarProps>) {
  const filters = descriptor.list?.filters ?? []
  // A runtime key lookup; a computed index would trip
  // `security/detect-object-injection`, which cannot be suppressed here.
  const valueOf = new Map(Object.entries(values))

  return (
    <div className="flex flex-wrap items-center gap-2">
      {filters.map(spec => (
        <ResourceFilterControl
          key={spec.key}
          spec={spec}
          value={valueOf.get(spec.key) ?? ''}
          onChange={next => {
            onChange(spec.key, next)
          }}
          descriptor={descriptor}
          searchInput={searchInput}
          onSearchInputChange={onSearchInputChange}
          metadata={metadata}
          onMetadataChange={onMetadataChange}
          projectId={projectId}
        />
      ))}

      {extras}

      {hasActiveFilters && onClear && (
        <Button variant="outline" onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
