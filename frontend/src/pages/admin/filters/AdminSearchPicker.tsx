import { Check, ChevronsUpDown, Loader2 } from 'lucide-react'
import type { UIEvent } from 'react'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
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
import type { AdminPagedSearchResult } from '@/pages/admin/filters/useAdminPagedSearch'

/** Sentinel value — never a real id (those are uuids). */
const ALL_VALUE = '__all__'
/** Distance from the bottom of the list (px) that pulls the next page. */
const SCROLL_LOAD_THRESHOLD = 48

export interface AdminSearchPickerProps<T> {
  /** Selected id, or '' for "all". */
  value: string
  onChange: (id: string) => void
  /** The server-side search behind the list (`useAdminPagedSearch`). */
  search: AdminPagedSearchResult<T>
  getId: (item: T) => string
  getLabel: (item: T) => string
  /** A second, muted line under the label (e.g. a user's email). */
  getDetail?: (item: T) => string | undefined
  /**
   * Resolves the label of a selected id that no loaded page contains — the
   * case when a shared link or a reload carries the id in the URL. Without it
   * the trigger reads `selectedFallback`.
   */
  resolveLabel?: (id: string) => Promise<string>
  /** Trigger text with nothing selected, e.g. "All teams". */
  allLabel: string
  /** Trigger text for a selected id whose label is unknown. */
  selectedFallback: string
  /** Placeholder of the search box, e.g. "Search teams…". */
  searchPlaceholder: string
  /** Empty-list text, e.g. "No teams found.". */
  emptyText: string
  'aria-label': string
  className?: string
}

/**
 * Searchable, paginated single-select for the admin filter bars: the team
 * picker and the audit log's user and project pickers (#1517).
 *
 * Mirrors `components/ProjectPicker` — `Popover` + `Command` over server-driven
 * search. Deliberately **not** a plain `Select`: filling one would mean
 * fetching every team (or user) on the instance.
 */
export function AdminSearchPicker<T>({
  value,
  onChange,
  search,
  getId,
  getLabel,
  getDetail,
  resolveLabel,
  allLabel,
  selectedFallback,
  searchPlaceholder,
  emptyText,
  'aria-label': ariaLabel,
  className,
}: Readonly<AdminSearchPickerProps<T>>) {
  const [open, setOpen] = useState(false)
  const { items, loading, loadingMore, error, hasMore, loadMore, query } =
    search

  // Remember the chosen item so the trigger keeps its label even when the
  // current search page no longer contains it.
  const [picked, setPicked] = useState<T | null>(null)
  // The label resolved for a selected id no page contains (a shared link).
  const [resolved, setResolved] = useState<{ id: string; label: string }>()

  const selected = useMemo(
    () =>
      items.find(item => getId(item) === value) ??
      (picked !== null && getId(picked) === value ? picked : null),
    [items, value, picked, getId]
  )

  useEffect(() => {
    if (!value) setPicked(null)
  }, [value])

  useEffect(() => {
    if (!value || selected !== null || resolveLabel === undefined) return
    if (resolved?.id === value) return
    let cancelled = false
    resolveLabel(value)
      .then(label => {
        if (!cancelled) setResolved({ id: value, label })
      })
      .catch(() => {
        // An id that no longer resolves (a deleted user) keeps the fallback.
      })
    return () => {
      cancelled = true
    }
  }, [value, selected, resolveLabel, resolved])

  let triggerLabel = allLabel
  if (value) {
    if (selected !== null) triggerLabel = getLabel(selected)
    else if (resolved?.id === value) triggerLabel = resolved.label
    else triggerLabel = selectedFallback
  }

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const el = event.currentTarget
    if (
      el.scrollHeight - el.scrollTop - el.clientHeight <
      SCROLL_LOAD_THRESHOLD
    ) {
      loadMore()
    }
  }

  const select = (item: T | null) => {
    setPicked(item)
    onChange(item === null ? '' : getId(item))
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          className={cn(
            'w-[200px] justify-between font-normal',
            !value && 'text-muted-foreground',
            className
          )}
        >
          <span className="truncate">{triggerLabel}</span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[260px] p-0" align="start">
        {/* shouldFilter={false}: the server already filtered, and filtering the
            loaded page again would hide results the next page would supply. */}
        <Command shouldFilter={false}>
          <CommandInput
            placeholder={searchPlaceholder}
            className="h-9"
            value={query}
            onValueChange={search.setQuery}
          />
          <CommandList className="max-h-64" onScroll={handleScroll}>
            {loading && (
              <div className="text-muted-foreground flex items-center justify-center gap-2 py-6 text-sm">
                <Loader2 className="size-4 animate-spin" />
                Searching…
              </div>
            )}
            {error && !loading && (
              <div className="text-destructive px-3 py-6 text-center text-sm">
                {error}
              </div>
            )}
            {!loading && !error && (
              <>
                <CommandItem
                  value={ALL_VALUE}
                  onSelect={() => {
                    select(null)
                  }}
                >
                  <Check
                    className={cn(
                      'mr-2 size-4',
                      value ? 'opacity-0' : 'opacity-100'
                    )}
                  />
                  {allLabel}
                </CommandItem>
                {items.length === 0 && <CommandEmpty>{emptyText}</CommandEmpty>}
                {items.map(item => {
                  const id = getId(item)
                  const detail = getDetail?.(item)
                  return (
                    <CommandItem
                      key={id}
                      value={id}
                      onSelect={() => {
                        select(item)
                      }}
                    >
                      <Check
                        className={cn(
                          'mr-2 size-4 shrink-0',
                          value === id ? 'opacity-100' : 'opacity-0'
                        )}
                      />
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate">{getLabel(item)}</span>
                        {detail && (
                          <span className="text-muted-foreground truncate text-xs">
                            {detail}
                          </span>
                        )}
                      </span>
                    </CommandItem>
                  )
                })}
                {loadingMore && (
                  <div className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-xs">
                    <Loader2 className="size-3 animate-spin" />
                    Loading more…
                  </div>
                )}
                {hasMore && !loadingMore && (
                  <div className="text-muted-foreground px-3 py-2 text-center text-xs">
                    Scroll for more
                  </div>
                )}
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
