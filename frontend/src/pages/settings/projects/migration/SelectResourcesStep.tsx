import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import type {
  MigrationInventory,
  ResourceInventory,
  ResourceSelection,
  ResourceSelections,
} from '@/services/projectMigrationService'

type ResourceKey = 'prompts' | 'artifacts' | 'blueprints' | 'feed_items'

const RESOURCE_LABELS: Record<ResourceKey, string> = {
  prompts: 'Prompts',
  artifacts: 'Artifacts',
  blueprints: 'Blueprints',
  feed_items: 'Feed Items',
}

interface ResourceSectionProps {
  resourceKey: ResourceKey
  label: string
  inventory: ResourceInventory
  selection: ResourceSelection
  defaultOpen: boolean
  onSelectAll: (checked: boolean) => void
  onToggleItem: (id: string, checked: boolean) => void
}

function ResourceSection({
  resourceKey,
  label,
  inventory,
  selection,
  defaultOpen,
  onSelectAll,
  onToggleItem,
}: ResourceSectionProps) {
  const [open, setOpen] = useState(defaultOpen)

  const items = inventory.items ?? []
  const selectedIds = new Set(selection.ids ?? [])
  const allChecked =
    (selection.all ?? false) ||
    (items.length > 0 && items.every(i => selectedIds.has(i.id)))
  const someChecked = !allChecked && items.some(i => selectedIds.has(i.id))
  const selectedCount = selection.all ? inventory.count : selectedIds.size

  const handleSelectAll = (checked: boolean | 'indeterminate') => {
    onSelectAll(checked === true)
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="rounded-lg border">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="hover:bg-muted/50 flex w-full items-center gap-3 p-4 text-left transition-colors"
            aria-expanded={open}
          >
            {open ? (
              <ChevronDown className="size-4 shrink-0" />
            ) : (
              <ChevronRight className="size-4 shrink-0" />
            )}
            <span className="flex-1 font-medium">{label}</span>
            <Badge variant={selectedCount > 0 ? 'default' : 'secondary'}>
              {selectedCount} / {inventory.count}
            </Badge>
          </button>
        </CollapsibleTrigger>

        <CollapsibleContent>
          <Separator />
          <div className="p-4 space-y-3">
            {inventory.count === 0 ? (
              <p className="text-muted-foreground text-sm">
                No {label.toLowerCase()} in this project.
              </p>
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={`select-all-${resourceKey}`}
                    checked={
                      allChecked ? true : someChecked ? 'indeterminate' : false
                    }
                    onCheckedChange={handleSelectAll}
                  />
                  <Label
                    htmlFor={`select-all-${resourceKey}`}
                    className="cursor-pointer text-sm font-medium"
                  >
                    Select all {label.toLowerCase()}
                  </Label>
                </div>

                {items.length > 0 && (
                  <div className="mt-2 space-y-2 pl-6">
                    {items.map(item => {
                      const checked =
                        (selection.all ?? false) || selectedIds.has(item.id)
                      return (
                        <div key={item.id} className="flex items-center gap-2">
                          <Checkbox
                            id={`${resourceKey}-${item.id}`}
                            checked={checked}
                            onCheckedChange={c => {
                              onToggleItem(item.id, c === true)
                            }}
                          />
                          <Label
                            htmlFor={`${resourceKey}-${item.id}`}
                            className="cursor-pointer text-sm"
                          >
                            {item.name}
                          </Label>
                        </div>
                      )
                    })}
                  </div>
                )}
              </>
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

interface SelectResourcesStepProps {
  inventory: MigrationInventory
  selectedResources: ResourceSelections
  onResourcesChange: (resources: ResourceSelections) => void
  onBack: () => void
  onNext: () => void
}

export function SelectResourcesStep({
  inventory,
  selectedResources,
  onResourcesChange,
  onBack,
  onNext,
}: SelectResourcesStepProps) {
  const resourceKeys: ResourceKey[] = [
    'prompts',
    'artifacts',
    'blueprints',
    'feed_items',
  ]

  const totalSelected = resourceKeys.reduce((acc, key) => {
    const sel = selectedResources[key]
    if (sel?.all) return acc + (inventory[key]?.count ?? 0)
    return acc + (sel?.ids?.length ?? 0)
  }, 0)

  const hasSelection = totalSelected > 0

  const handleSelectAll = (key: ResourceKey, checked: boolean) => {
    const items = inventory[key]?.items ?? []
    onResourcesChange({
      ...selectedResources,
      [key]: checked
        ? { all: true, ids: items.map(i => i.id) }
        : { all: false, ids: [] },
    })
  }

  const handleToggleItem = (key: ResourceKey, id: string, checked: boolean) => {
    const current = selectedResources[key]
    const currentIds = new Set(current?.ids ?? [])

    if (checked) {
      currentIds.add(id)
    } else {
      currentIds.delete(id)
    }

    const items = inventory[key]?.items ?? []
    const allSelected =
      items.length > 0 && items.every(i => currentIds.has(i.id))

    onResourcesChange({
      ...selectedResources,
      [key]: {
        all: allSelected,
        ids: Array.from(currentIds),
      },
    })
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Select resources to migrate</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Choose which resources to move to the destination project. Feed items
          are unchecked by default.
        </p>
      </div>

      <div className="space-y-3">
        {resourceKeys.map((key, idx) => (
          <ResourceSection
            key={key}
            resourceKey={key}
            label={RESOURCE_LABELS[key]}
            inventory={inventory[key] ?? { count: 0 }}
            selection={selectedResources[key] ?? {}}
            defaultOpen={idx < 3}
            onSelectAll={checked => {
              handleSelectAll(key, checked)
            }}
            onToggleItem={(id, checked) => {
              handleToggleItem(key, id, checked)
            }}
          />
        ))}
      </div>

      <div className="flex items-center justify-between">
        <p className="text-muted-foreground text-sm">
          {hasSelection
            ? `${String(totalSelected)} resource${totalSelected === 1 ? '' : 's'} selected`
            : 'No resources selected'}
        </p>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onBack}>
            Back
          </Button>
          <Button onClick={onNext} disabled={!hasSelection}>
            Next
          </Button>
        </div>
      </div>
    </div>
  )
}
