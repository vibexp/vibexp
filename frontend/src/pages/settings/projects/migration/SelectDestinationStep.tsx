import { AlertTriangle, Check, ChevronsUpDown, Loader2 } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Separator } from '@/components/ui/separator'
import { useProjectSearch } from '@/hooks'
import { cn } from '@/lib/utils'
import type { Project } from '@/types/project'
import type {
  ConflictPolicy,
  MigrationResources,
} from '@/types/projectMigration'

interface ConflictPolicyOption {
  value: ConflictPolicy
  label: string
  description: string
}

const CONFLICT_POLICIES: ConflictPolicyOption[] = [
  {
    value: 'skip',
    label: 'Skip',
    description: 'Leave resources with conflicting names in the source project',
  },
  {
    value: 'rename',
    label: 'Rename',
    description: "Add '-moved' suffix to resolve naming conflicts",
  },
  {
    value: 'overwrite',
    label: 'Overwrite',
    description: 'Replace destination resources that have conflicting names',
  },
]

interface ResourceSummaryRowProps {
  label: string
  count: number
}

function ResourceSummaryRow({ label, count }: ResourceSummaryRowProps) {
  if (count === 0) return null
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{count}</span>
    </div>
  )
}

function countSelected(
  sel: MigrationResources['prompts'],
  total: number
): number {
  if (sel.all) return total
  return sel.ids?.length ?? 0
}

interface SelectDestinationStepProps {
  sourceProjectId: string
  sourceProjectName: string
  destinationProjectId: string
  conflictPolicy: ConflictPolicy
  selectedResources: MigrationResources
  inventoryCounts: {
    prompts: number
    artifacts: number
    blueprints: number
    feed_items: number
  }
  onDestinationSelect: (project: Project) => void
  onConflictPolicyChange: (policy: ConflictPolicy) => void
  onBack: () => void
  onMigrate: () => void
  migrating: boolean
}

export function SelectDestinationStep({
  sourceProjectId,
  sourceProjectName,
  destinationProjectId,
  conflictPolicy,
  selectedResources,
  inventoryCounts,
  onDestinationSelect,
  onConflictPolicyChange,
  onBack,
  onMigrate,
  migrating,
}: SelectDestinationStepProps) {
  const [open, setOpen] = useState(false)
  // The chosen destination may not be in the current search page, so remember
  // it locally to keep the summary populated as the query changes.
  const [selectedDestination, setSelectedDestination] =
    useState<Project | null>(null)
  const { projects, loading, error, query, setQuery } = useProjectSearch({
    excludeProjectId: sourceProjectId,
  })

  const destination =
    projects.find(p => p.id === destinationProjectId) ??
    (selectedDestination?.id === destinationProjectId
      ? selectedDestination
      : undefined)

  const promptCount = countSelected(
    selectedResources.prompts,
    inventoryCounts.prompts
  )
  const artifactCount = countSelected(
    selectedResources.artifacts,
    inventoryCounts.artifacts
  )
  const blueprintCount = countSelected(
    selectedResources.blueprints,
    inventoryCounts.blueprints
  )
  const feedItemCount = countSelected(
    selectedResources.feed_items,
    inventoryCounts.feed_items
  )

  const canMigrate = !!destinationProjectId && !migrating

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">
          Select destination &amp; policy
        </h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Choose where to move the resources and how to handle naming conflicts.
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="destination-trigger">Destination project</Label>
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              id="destination-trigger"
              variant="outline"
              role="combobox"
              aria-expanded={open}
              className="w-full justify-between"
            >
              <span className="truncate">
                {destination ? destination.name : 'Select destination project…'}
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
                    <CommandEmpty>No other projects found.</CommandEmpty>
                    <CommandGroup>
                      {projects.map(project => (
                        <CommandItem
                          key={project.id}
                          value={project.id}
                          onSelect={() => {
                            setSelectedDestination(project)
                            onDestinationSelect(project)
                            setOpen(false)
                          }}
                        >
                          <Check
                            className={cn(
                              'mr-2 size-4',
                              destinationProjectId === project.id
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

      <div className="space-y-3">
        <Label>Conflict policy</Label>
        <div
          className="space-y-2"
          role="radiogroup"
          aria-label="Conflict policy"
        >
          {CONFLICT_POLICIES.map(option => (
            <div
              key={option.value}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition-colors',
                conflictPolicy === option.value
                  ? 'border-primary bg-primary/5'
                  : 'hover:bg-muted/50'
              )}
            >
              <input
                id={`conflict-policy-${option.value}`}
                type="radio"
                name="conflict-policy"
                value={option.value}
                checked={conflictPolicy === option.value}
                onChange={() => {
                  onConflictPolicyChange(option.value)
                }}
                className="mt-0.5 size-4 accent-primary"
              />
              <label
                htmlFor={`conflict-policy-${option.value}`}
                className="cursor-pointer"
              >
                <p className="text-sm font-medium">{option.label}</p>
                <p className="text-muted-foreground text-sm">
                  {option.description}
                </p>
              </label>
            </div>
          ))}
        </div>
      </div>

      {conflictPolicy === 'overwrite' && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertDescription>
            Overwrite will permanently replace resources in the destination
            project that share names with migrated resources. This cannot be
            undone.
          </AlertDescription>
        </Alert>
      )}

      {destination && (
        <div className="rounded-lg border p-4 space-y-3">
          <p className="text-sm font-medium">Migration summary</p>
          <Separator />
          <div className="space-y-1">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">From</span>
              <span className="font-medium">{sourceProjectName}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">To</span>
              <span className="font-medium">{destination.name}</span>
            </div>
          </div>
          <Separator />
          <div className="space-y-1">
            <ResourceSummaryRow label="Prompts" count={promptCount} />
            <ResourceSummaryRow label="Artifacts" count={artifactCount} />
            <ResourceSummaryRow label="Blueprints" count={blueprintCount} />
            <ResourceSummaryRow label="Feed items" count={feedItemCount} />
          </div>
          <Separator />
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">On conflict</span>
            <span className="font-medium capitalize">{conflictPolicy}</span>
          </div>
        </div>
      )}

      <div className="flex justify-between">
        <Button variant="outline" onClick={onBack} disabled={migrating}>
          Back
        </Button>
        <Button onClick={onMigrate} disabled={!canMigrate}>
          {migrating ? 'Migrating…' : 'Migrate'}
        </Button>
      </div>
    </div>
  )
}
