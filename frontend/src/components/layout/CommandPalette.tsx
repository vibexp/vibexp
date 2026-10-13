import { ArrowRight, Search as SearchIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'

import { NAV_ITEMS } from '@/components/layout/nav-items'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog'
import { useTeam } from '@/contexts/TeamContext'
import { RESOURCE_TYPE_META } from '@/pages/home/resourceTypeMeta'
import { displayTitle, resourceUrl } from '@/pages/search/resourceUrl'
import { type SearchResultItem, searchService } from '@/services/searchService'

import {
  groupResults,
  type ResultGroup,
  searchPagePath,
} from './commandPaletteResults'

/** How long typing must pause before a search is sent. */
export const PALETTE_DEBOUNCE_MS = 250
/** Shorter queries match too much to be worth a request. */
const MIN_QUERY_LENGTH = 2
/** One page of hits; the palette lists resources, not every matching chunk. */
const PALETTE_PAGE_SIZE = 12

type SearchState =
  | { status: 'idle' }
  | { status: 'loading'; query: string }
  | { status: 'done'; query: string; groups: ResultGroup[] }
  | { status: 'error'; query: string }

/**
 * Runs the palette's search: debounced, and stale-guarded so a slow response
 * for an earlier query can never replace the results of a later one.
 */
function usePaletteSearch(teamId: string | undefined, query: string) {
  const [state, setState] = useState<SearchState>({ status: 'idle' })
  const searchable = teamId !== undefined && query.length >= MIN_QUERY_LENGTH

  useEffect(() => {
    if (!searchable) return
    let cancelled = false
    const timer = setTimeout(() => {
      setState({ status: 'loading', query })
      searchService
        .search(teamId, { query, page: 1, per_page: PALETTE_PAGE_SIZE })
        .then(response => {
          if (cancelled) return
          setState({
            status: 'done',
            query,
            groups: groupResults(response.results),
          })
        })
        .catch((error: unknown) => {
          console.warn('Command palette search failed:', error)
          if (!cancelled) setState({ status: 'error', query })
        })
    }, PALETTE_DEBOUNCE_MS)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [searchable, teamId, query])

  // Results for anything but the query on screen are not results.
  return searchable ? state : ({ status: 'idle' } as const)
}

const SEE_ALL_VALUE = 'see-all-results'
const navValue = (href: string): string => `nav:${href}`
const resultValue = (item: SearchResultItem): string =>
  `${item.type}:${item.id}`

/** The value of the first row of the list currently on screen. */
function firstRowValue(query: string, search: SearchState): string {
  if (query.length < MIN_QUERY_LENGTH) return navValue(NAV_ITEMS[0].href)
  if (search.status === 'done' && search.groups.length > 0) {
    return resultValue(search.groups[0].items[0])
  }
  return SEE_ALL_VALUE
}

const isMac = (): boolean =>
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

/**
 * The header's search: a command palette.
 *
 * One line of input, results as you type (grouped by type, each with its
 * project), arrow keys to move, Enter to open, and a last row that takes the
 * query to the full `/search` page. With nothing typed it is quick navigation
 * to the main pages. Opened by the header button and by Ctrl/Cmd+K; Escape
 * closes it.
 *
 * cmdk supplies the list semantics and the keyboard. Its own filtering is
 * off: the results are the server's ranking, not a client-side fuzzy match of
 * what happens to be loaded. The AI summary stays on the full search page.
 */
export function CommandPalette() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState('')
  const query = value.trim()
  const search = usePaletteSearch(currentTeam?.id, query)

  // The highlighted row. cmdk keeps the highlight on a row only while that
  // row exists, so when the list is replaced (results arrive, the query is
  // cleared) nothing would be highlighted and Enter would do nothing. The
  // first row of whatever list is showing is highlighted instead, so Enter
  // always opens the top hit.
  const [active, setActive] = useState('')
  const firstRow = firstRowValue(query, search)
  useEffect(() => {
    setActive(firstRow)
  }, [firstRow])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() !== 'k') return
      if (!(event.metaKey || event.ctrlKey) || event.altKey) return
      event.preventDefault()
      setOpen(current => !current)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [])

  const handleOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) setValue('')
  }

  const go = (to: string) => {
    handleOpenChange(false)
    void navigate(to)
  }

  const shortcut = isMac() ? '⌘K' : 'Ctrl+K'

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Search"
        aria-keyshortcuts="Control+K Meta+K"
        title={`Search (${shortcut})`}
        data-testid="command-palette-trigger"
        onClick={() => {
          setOpen(true)
        }}
      >
        <SearchIcon className="size-5" />
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent
          className="top-[20%] translate-y-0 overflow-hidden p-0 sm:max-w-xl"
          data-testid="command-palette"
        >
          <DialogTitle className="sr-only">Search</DialogTitle>
          <DialogDescription className="sr-only">
            Search prompts, artifacts, blueprints and memories, or jump to a
            page.
          </DialogDescription>
          <Command
            shouldFilter={false}
            loop
            label="Search"
            value={active}
            onValueChange={setActive}
          >
            <CommandInput
              value={value}
              onValueChange={setValue}
              placeholder="Search, or jump to a page…"
            />
            <CommandList className="max-h-[min(60dvh,26rem)]">
              {query.length < MIN_QUERY_LENGTH ? (
                <CommandGroup heading="Go to">
                  {NAV_ITEMS.map(item => (
                    <CommandItem
                      key={item.href}
                      value={navValue(item.href)}
                      onSelect={() => {
                        go(item.href)
                      }}
                    >
                      <item.icon className="text-muted-foreground mr-2 size-4" />
                      {item.label}
                    </CommandItem>
                  ))}
                </CommandGroup>
              ) : (
                <>
                  <PaletteResults search={search} onOpen={go} />
                  <CommandGroup>
                    <CommandItem
                      value={SEE_ALL_VALUE}
                      onSelect={() => {
                        go(searchPagePath(query))
                      }}
                      data-testid="command-palette-see-all"
                    >
                      <ArrowRight className="text-muted-foreground mr-2 size-4" />
                      <span className="truncate">
                        See all results for “{query}”
                      </span>
                    </CommandItem>
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </DialogContent>
      </Dialog>
    </>
  )
}

function PaletteResults({
  search,
  onOpen,
}: Readonly<{ search: SearchState; onOpen: (to: string) => void }>) {
  if (search.status === 'idle' || search.status === 'loading') {
    return (
      <p
        className="text-muted-foreground px-4 py-3 text-sm"
        role="status"
        aria-live="polite"
      >
        Searching…
      </p>
    )
  }
  if (search.status === 'error') {
    return (
      <p className="text-muted-foreground px-4 py-3 text-sm" role="status">
        Search is unavailable right now.
      </p>
    )
  }
  if (search.groups.length === 0) {
    return (
      <p className="text-muted-foreground px-4 py-3 text-sm" role="status">
        No matches.
      </p>
    )
  }
  return (
    <>
      {search.groups.map(group => {
        const Icon = RESOURCE_TYPE_META[group.type].icon
        return (
          <CommandGroup key={group.type} heading={group.heading}>
            {group.items.map(item => {
              const url = resourceUrl(item)
              return (
                <CommandItem
                  key={resultValue(item)}
                  value={resultValue(item)}
                  disabled={url === null}
                  onSelect={() => {
                    if (url) onOpen(url)
                  }}
                >
                  <Icon className="text-muted-foreground mr-2 size-4 shrink-0" />
                  <span className="min-w-0 flex-1 truncate">
                    {displayTitle(item)}
                  </span>
                  {item.project_name && (
                    <span className="text-muted-foreground ml-3 shrink-0 truncate text-xs">
                      {item.project_name}
                    </span>
                  )}
                </CommandItem>
              )
            })}
          </CommandGroup>
        )
      })}
    </>
  )
}
