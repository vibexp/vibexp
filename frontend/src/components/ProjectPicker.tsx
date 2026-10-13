import { Check, ChevronsUpDown, Loader2 } from 'lucide-react'
import type { AriaAttributes, UIEvent } from 'react'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useTeam } from '@/contexts/TeamContext'
import { useProjectSearch } from '@/hooks'
import { cn } from '@/lib/utils'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'

// Sentinel CommandItem values — never used as real project ids (uuids).
const ALL_OPTION_VALUE = '__all_projects__'
const LOAD_MORE_VALUE = '__load_more__'
// Trigger near the bottom of the scroll viewport (px) to pull the next page.
const SCROLL_LOAD_THRESHOLD = 48

interface ProjectPickerProps extends AriaAttributes {
  /** Selected project id, or null/empty when nothing (or "All projects") is selected. */
  value: string | null
  /**
   * Called with the chosen project id, or `null` when "All projects" is
   * picked. The full `Project` is passed alongside the id for consumers that
   * need more than the id (e.g. the header switcher storing the selection).
   */
  onChange: (projectId: string | null, project: Project | null) => void
  /** Render a leading option that clears the selection (for list filters). */
  includeAllOption?: boolean
  /** Label for the "All projects" option and the cleared-state trigger. */
  allOptionLabel?: string
  /** Trigger placeholder shown when nothing is selected. */
  placeholder?: string
  disabled?: boolean
  /**
   * Optional pre-resolved project for the current value, so the trigger shows
   * the right name even before that project appears in a loaded search page.
   */
  selectedProject?: Project | null
  id?: string
  triggerClassName?: string
  /**
   * Put the trigger's label in its `title` too, for a compact trigger that
   * truncates the name (the edit page's Project row, #1180).
   */
  titleTrigger?: boolean
  'data-testid'?: string
}

/**
 * A preselected project the loaded page does not hold (a default seeded by id,
 * or one past the first page) is looked up by id, so the trigger names it
 * instead of falling back to a generic label. Returns the label to show until
 * then.
 */
function useProjectLookup({
  projectId,
  listSettled,
  onResolved,
}: {
  projectId: string | null
  listSettled: boolean
  onResolved: (project: Project) => void
}): { label: string } {
  const { currentTeam } = useTeam()
  const teamId = currentTeam?.id
  const [unknownId, setUnknownId] = useState<string | null>(null)

  // The lookup waits for the first page, which usually answers the question
  // without a second request.
  const lookupId = listSettled ? projectId : null

  useEffect(() => {
    if (!teamId || !lookupId) return
    let cancelled = false
    const resolve = async () => {
      try {
        const project = await projectService.getProject(teamId, lookupId)
        if (cancelled) return
        if (project.id === lookupId) onResolved(project)
        else setUnknownId(lookupId)
      } catch {
        if (!cancelled) setUnknownId(lookupId)
      }
    }
    void resolve()
    return () => {
      cancelled = true
    }
  }, [teamId, lookupId, onResolved])

  return {
    label: unknownId === projectId ? 'Selected project' : 'Loading project…',
  }
}

/**
 * Reusable, searchable, paginated, scrollable project picker. Wraps a
 * `Popover` + `Command` combobox over server-driven search ({@link useProjectSearch})
 * with a height-bounded, scrollable result list that loads more pages on scroll.
 */
export function ProjectPicker({
  value,
  onChange,
  includeAllOption = false,
  allOptionLabel = 'All projects',
  placeholder = 'Select project…',
  disabled = false,
  selectedProject = null,
  id,
  triggerClassName,
  titleTrigger = false,
  'data-testid': dataTestId,
  // Forwarded onto the trigger so a wrapping <FormControl> can wire
  // aria-invalid / aria-describedby to the field's error message.
  ...ariaProps
}: Readonly<ProjectPickerProps>) {
  const [open, setOpen] = useState(false)
  const {
    projects,
    loading,
    settled,
    loadingMore,
    error,
    hasMore,
    loadMore,
    query,
    setQuery,
  } = useProjectSearch()

  // Remember the last project the user picked (seeded from selectedProject) so
  // the trigger label survives even when the active search page omits it.
  const [picked, setPicked] = useState<Project | null>(selectedProject)
  useEffect(() => {
    if (selectedProject) setPicked(selectedProject)
  }, [selectedProject])

  const isAllSelected = includeAllOption && !value

  const selected = useMemo(() => {
    if (!value) return null
    return (
      projects.find(p => p.id === value) ??
      (picked?.id === value ? picked : null)
    )
  }, [projects, value, picked])

  const lookup = useProjectLookup({
    projectId: value && !selected ? value : null,
    listSettled: settled && !loading,
    onResolved: setPicked,
  })
  const fallbackLabel = value ? lookup.label : placeholder
  const triggerLabel = isAllSelected
    ? allOptionLabel
    : (selected?.name ?? fallbackLabel)

  const handleSelectProject = (project: Project) => {
    setPicked(project)
    onChange(project.id, project)
    setOpen(false)
  }

  const handleSelectAll = () => {
    setPicked(null)
    onChange(null, null)
    setOpen(false)
  }

  // Infinite scroll: pull the next page as the viewport nears the bottom.
  // loadMore() is internally guarded, so calling it when busy/exhausted is safe.
  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const el = event.currentTarget
    if (
      el.scrollHeight - el.scrollTop - el.clientHeight <
      SCROLL_LOAD_THRESHOLD
    ) {
      loadMore()
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          {...ariaProps}
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          data-testid={dataTestId}
          title={titleTrigger ? triggerLabel : undefined}
          className={cn(
            'w-full justify-between font-normal',
            !selected && !isAllSelected && 'text-muted-foreground',
            triggerClassName
          )}
        >
          <span className="truncate">{triggerLabel}</span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[--radix-popover-trigger-width] p-0"
        align="start"
      >
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search projects…"
            className="h-9"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList className="max-h-72" onScroll={handleScroll}>
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
                <CommandEmpty>No projects found.</CommandEmpty>
                <CommandGroup>
                  {includeAllOption && (
                    <CommandItem
                      value={ALL_OPTION_VALUE}
                      onSelect={handleSelectAll}
                    >
                      <Check
                        className={cn(
                          'mr-2 size-4',
                          isAllSelected ? 'opacity-100' : 'opacity-0'
                        )}
                      />
                      <span className="truncate">{allOptionLabel}</span>
                    </CommandItem>
                  )}
                  {projects.map(project => (
                    <CommandItem
                      key={project.id}
                      value={project.id}
                      onSelect={() => {
                        handleSelectProject(project)
                      }}
                    >
                      <Check
                        className={cn(
                          'mr-2 size-4',
                          value === project.id ? 'opacity-100' : 'opacity-0'
                        )}
                      />
                      <span className="truncate">{project.name}</span>
                      <code className="bg-muted ml-2 rounded px-1.5 py-0.5 font-mono text-xs">
                        {project.slug}
                      </code>
                    </CommandItem>
                  ))}
                  {loadingMore && (
                    <div className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-sm">
                      <Loader2 className="size-4 animate-spin" />
                      Loading more…
                    </div>
                  )}
                  {hasMore && !loadingMore && (
                    <CommandItem
                      value={LOAD_MORE_VALUE}
                      onSelect={() => {
                        loadMore()
                      }}
                      className="text-muted-foreground justify-center text-sm"
                    >
                      Load more
                    </CommandItem>
                  )}
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
