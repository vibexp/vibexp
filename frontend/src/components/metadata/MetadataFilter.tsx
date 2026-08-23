import { ChevronsUpDown, Loader2, Plus, X } from 'lucide-react'
import { useCallback, useEffect, useEffectEvent, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import type { MetadataFilterValue } from '@/services/metadataService'

/**
 * A controlled metadata filter: a `+ Metadata` popover for picking one key and
 * multi-selecting its values, plus a removable chip per committed key.
 *
 * The emitted value is the same JSON-serialisable `Record<string, string[]>`
 * the `metadata` query parameter takes, so a page can hand it straight to its
 * service. Keys are ANDed and values within a key ORed, matching the backend.
 *
 * Deliberately domain-free and fetch-free — the catalog (keys, values, loading
 * and error state) arrives via props, normally from `useMetadataCatalog`. That
 * keeps it promotable into the design system unchanged, the same contract
 * `ListPage` and `DateRangePicker` carry.
 *
 * Committing with no values selected REMOVES the key rather than emitting an
 * empty array — `{"env":[]}` means "the key exists" to the backend, which is
 * not what an empty checkbox list expresses.
 */
/** Returns the filter without `key`, leaving the original untouched. */
function omitKey(value: MetadataFilterValue, key: string): MetadataFilterValue {
  return Object.fromEntries(
    Object.entries(value).filter(([entry]) => entry !== key)
  )
}

export interface MetadataFilterProps {
  /** The committed filter. One chip renders per key. */
  value: MetadataFilterValue
  /** Fires with the next filter whenever a chip is added, edited or removed. */
  onChange: (value: MetadataFilterValue) => void
  /** Metadata keys in use, for the key list. */
  keys: string[]
  keysLoading?: boolean
  keysError?: string | null
  /** Called when the popover opens, so the host can load the key catalog lazily. */
  onOpenCatalog?: () => void
  /** The key whose values are currently listed, or null while picking a key. */
  activeKey: string | null
  /** Selects the key to list values for; null returns to the key list. */
  onSelectKey: (key: string | null) => void
  /** Values available for `activeKey`. */
  values: string[]
  valuesLoading?: boolean
  valuesError?: string | null
  /** True when more values exist than were returned. */
  valuesTruncated?: boolean
  /** Current value-search text. */
  valueQuery: string
  onValueQueryChange: (query: string) => void
  disabled?: boolean
  className?: string
  /** Accessible name for the trigger, e.g. "Filter blueprints by metadata". */
  ariaLabel?: string
}

export function MetadataFilter({
  value,
  onChange,
  keys,
  keysLoading = false,
  keysError = null,
  onOpenCatalog,
  activeKey,
  onSelectKey,
  values,
  valuesLoading = false,
  valuesError = null,
  valuesTruncated = false,
  valueQuery,
  onValueQueryChange,
  disabled = false,
  className,
  ariaLabel = 'Filter by metadata',
}: Readonly<MetadataFilterProps>) {
  const [open, setOpen] = useState(false)
  const [draftValues, setDraftValues] = useState<string[]>([])

  // The host owns activeKey, so seed the draft from whatever key it hands us
  // rather than from the click that requested it — otherwise re-opening on an
  // existing chip would start with nothing ticked. An effect event reads the
  // latest `value` without making it a dependency, so a re-seed happens only
  // when the key actually changes, never on top of in-progress user toggles.
  const seedDraftFromValue = useEffectEvent((key: string | null) => {
    setDraftValues(key === null ? [] : (value[key] ?? []))
  })

  useEffect(() => {
    seedDraftFromValue(activeKey)
  }, [activeKey])

  const activeKeys = Object.keys(value)

  const handleOpenChange = useCallback(
    (next: boolean) => {
      setOpen(next)
      if (next) {
        onOpenCatalog?.()
      } else {
        onSelectKey(null)
        setDraftValues([])
      }
    },
    [onOpenCatalog, onSelectKey]
  )

  const handleSelectKey = useCallback(
    (key: string) => {
      // The seeding effect above picks the draft up from the new activeKey.
      onSelectKey(key)
    },
    [onSelectKey]
  )

  const toggleDraftValue = useCallback((candidate: string) => {
    setDraftValues(current =>
      current.includes(candidate)
        ? current.filter(entry => entry !== candidate)
        : [...current, candidate]
    )
  }, [])

  const applyDraft = useCallback(() => {
    if (!activeKey) return

    const next =
      draftValues.length === 0
        ? omitKey(value, activeKey)
        : { ...value, [activeKey]: draftValues }

    onChange(next)
    setOpen(false)
    onSelectKey(null)
    setDraftValues([])
  }, [activeKey, draftValues, onChange, onSelectKey, value])

  const removeKey = useCallback(
    (key: string) => {
      onChange(omitKey(value, key))
    },
    [onChange, value]
  )

  return (
    <div className={cn('flex flex-wrap items-center gap-2', className)}>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            role="combobox"
            aria-expanded={open}
            aria-label={ariaLabel}
            disabled={disabled}
            data-testid="metadata-filter-trigger"
          >
            <Plus className="mr-1 size-4" />
            Metadata
            <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>

        <PopoverContent className="w-72 p-0" align="start">
          {activeKey === null ? (
            <MetadataKeyList
              keys={keys}
              loading={keysLoading}
              error={keysError}
              onSelect={handleSelectKey}
            />
          ) : (
            <MetadataValueList
              activeKey={activeKey}
              values={values}
              draftValues={draftValues}
              loading={valuesLoading}
              error={valuesError}
              truncated={valuesTruncated}
              query={valueQuery}
              onQueryChange={onValueQueryChange}
              onToggle={toggleDraftValue}
              onApply={applyDraft}
              onBack={() => {
                onSelectKey(null)
                setDraftValues([])
              }}
            />
          )}
        </PopoverContent>
      </Popover>

      {activeKeys.map(key => (
        <Badge
          key={key}
          variant="secondary"
          className="gap-1"
          data-testid={`metadata-chip-${key}`}
        >
          <span>
            {/* An empty array is the backend's "key exists" form, which a page
                may restore from the URL — render it as such, not as a blank. */}
            {value[key].length > 0
              ? `${key}: ${value[key].join(', ')}`
              : `${key}: any`}
          </span>
          <button
            type="button"
            aria-label={`Remove ${key} filter`}
            className="ml-1 rounded-sm opacity-70 hover:opacity-100"
            onClick={() => {
              removeKey(key)
            }}
          >
            <X className="size-3" />
          </button>
        </Badge>
      ))}

      {activeKeys.length > 0 && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label="Clear metadata filters"
          onClick={() => {
            onChange({})
          }}
        >
          Clear filters
        </Button>
      )}
    </div>
  )
}

interface MetadataKeyListProps {
  keys: string[]
  loading: boolean
  error: string | null
  onSelect: (key: string) => void
}

function MetadataKeyList({
  keys,
  loading,
  error,
  onSelect,
}: Readonly<MetadataKeyListProps>) {
  return (
    <Command shouldFilter>
      <CommandInput
        placeholder="Search keys…"
        className="h-9"
        aria-label="Search metadata keys"
      />
      <CommandList className="max-h-72">
        {loading && (
          <div className="flex items-center justify-center py-6">
            <Loader2 className="size-4 animate-spin" />
          </div>
        )}
        {!loading && error !== null && (
          <div className="px-3 py-6 text-center text-sm text-destructive">
            {error}
          </div>
        )}
        {!loading && error === null && (
          <>
            <CommandEmpty>No metadata keys found.</CommandEmpty>
            {keys.map(key => (
              <CommandItem
                key={key}
                value={key}
                onSelect={() => {
                  onSelect(key)
                }}
              >
                {key}
              </CommandItem>
            ))}
          </>
        )}
      </CommandList>
    </Command>
  )
}

interface MetadataValueListProps {
  activeKey: string
  values: string[]
  draftValues: string[]
  loading: boolean
  error: string | null
  truncated: boolean
  query: string
  onQueryChange: (query: string) => void
  onToggle: (value: string) => void
  onApply: () => void
  onBack: () => void
}

function MetadataValueList({
  activeKey,
  values,
  draftValues,
  loading,
  error,
  truncated,
  query,
  onQueryChange,
  onToggle,
  onApply,
  onBack,
}: Readonly<MetadataValueListProps>) {
  return (
    // shouldFilter={false}: the search is server-side, so cmdk must not also
    // filter the values it is given.
    <Command shouldFilter={false}>
      <CommandInput
        placeholder={`Search ${activeKey} values…`}
        className="h-9"
        aria-label={`Search ${activeKey} values`}
        value={query}
        onValueChange={onQueryChange}
      />
      <CommandList className="max-h-64">
        {loading && (
          <div className="flex items-center justify-center py-6">
            <Loader2 className="size-4 animate-spin" />
          </div>
        )}
        {!loading && error !== null && (
          <div className="px-3 py-6 text-center text-sm text-destructive">
            {error}
          </div>
        )}
        {!loading && error === null && (
          <>
            <CommandEmpty>No values found.</CommandEmpty>
            {values.map(entry => (
              // onSelect toggles instead of committing, so the list stays a
              // multi-select; only Apply closes the popover.
              <CommandItem
                key={entry}
                value={entry}
                onSelect={() => {
                  onToggle(entry)
                }}
              >
                <Checkbox
                  checked={draftValues.includes(entry)}
                  aria-label={entry}
                  className="mr-2"
                  tabIndex={-1}
                />
                {entry}
              </CommandItem>
            ))}
            {truncated && (
              <div
                className="px-3 py-2 text-xs text-muted-foreground"
                role="status"
              >
                More values available — keep typing to narrow.
              </div>
            )}
          </>
        )}
      </CommandList>

      <div className="flex items-center justify-between border-t p-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label="Back to keys"
          onClick={onBack}
        >
          Back
        </Button>
        <Button
          type="button"
          size="sm"
          aria-label={`Apply ${activeKey} filter`}
          onClick={onApply}
        >
          Apply
        </Button>
      </div>
    </Command>
  )
}
