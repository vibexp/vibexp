import { Check, ChevronsUpDown, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useProjectSearch } from '@/hooks'
import { cn } from '@/lib/utils'
import type { Project } from '@/services/projectService'

interface SelectSourceStepProps {
  /**
   * The source project resolved from the URL slug. Kept selectable even when
   * it isn't part of the current search results.
   */
  resolvedSourceProject: Project | null
  selectedProjectId: string
  onSelect: (project: Project) => void
  onNext: () => void
  loadingInventory: boolean
}

export function SelectSourceStep({
  resolvedSourceProject,
  selectedProjectId,
  onSelect,
  onNext,
  loadingInventory,
}: Readonly<SelectSourceStepProps>) {
  const [open, setOpen] = useState(false)
  const { projects, loading, error, query, setQuery } = useProjectSearch()

  // Surface the URL-resolved source project even if the current search page
  // doesn't include it, so it stays visible and selected.
  const visibleProjects = useMemo(() => {
    if (
      resolvedSourceProject &&
      !projects.some(p => p.id === resolvedSourceProject.id)
    ) {
      return [resolvedSourceProject, ...projects]
    }
    return projects
  }, [resolvedSourceProject, projects])

  const selected =
    visibleProjects.find(p => p.id === selectedProjectId) ??
    (resolvedSourceProject?.id === selectedProjectId
      ? resolvedSourceProject
      : undefined)

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Select source project</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Choose the project whose resources you want to migrate. The project
          from your URL is pre-selected.
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="source-project-trigger">Source project</Label>
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              id="source-project-trigger"
              variant="outline"
              role="combobox"
              aria-expanded={open}
              className="w-full justify-between"
            >
              <span className="truncate">
                {selected ? selected.name : 'Select project…'}
              </span>
              <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[400px] p-0" align="start">
            <Command shouldFilter={false}>
              <CommandInput
                placeholder="Search projects…"
                className="h-9"
                value={query}
                onValueChange={setQuery}
              />
              <CommandList>
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
                      {visibleProjects.map(project => (
                        <CommandItem
                          key={project.id}
                          value={project.id}
                          onSelect={() => {
                            onSelect(project)
                            setOpen(false)
                          }}
                        >
                          <Check
                            className={cn(
                              'mr-2 size-4',
                              selectedProjectId === project.id
                                ? 'opacity-100'
                                : 'opacity-0'
                            )}
                          />
                          <span className="truncate">{project.name}</span>
                          <code className="bg-muted ml-2 rounded px-1.5 py-0.5 font-mono text-xs">
                            {project.slug}
                          </code>
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </>
                )}
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>

      <div className="flex justify-end">
        <Button
          onClick={onNext}
          disabled={!selectedProjectId || loadingInventory}
        >
          {loadingInventory ? (
            <>
              <Loader2 className="mr-2 size-4 animate-spin" />
              Loading inventory…
            </>
          ) : (
            'Next'
          )}
        </Button>
      </div>
    </div>
  )
}
