import { Search } from 'lucide-react'

import { MetadataFilter } from '@/components/metadata/MetadataFilter'
import { useMetadataCatalog } from '@/components/metadata/useMetadataCatalog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Blueprint } from '@/services/blueprintService'
import type { MetadataFilterValue } from '@/services/metadataService'

export interface BlueprintFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  type: Blueprint['type'] | undefined
  onTypeChange: (value: Blueprint['type'] | undefined) => void
  /** Committed metadata filter; one chip per key. */
  metadata: MetadataFilterValue
  onMetadataChange: (value: MetadataFilterValue) => void
  /** Narrows the metadata catalog to the globally selected project. */
  projectId?: string
  /** Shown only while at least one filter is applied. */
  onClear?: () => void
  hasActiveFilters: boolean
}

// Project filtering moved to the global header project selector (useProject).
export function BlueprintFilters({
  searchInput,
  onSearchInputChange,
  type,
  onTypeChange,
  metadata,
  onMetadataChange,
  projectId,
  onClear,
  hasActiveFilters,
}: Readonly<BlueprintFiltersProps>) {
  // The catalog is this bar's own concern: MetadataFilter stays fetch-free so
  // it can move into the design system, and the page stays free of catalog
  // state it never reads.
  const catalog = useMetadataCatalog({
    resourceType: 'blueprints',
    projectId,
  })

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search blueprints…"
          aria-label="Search blueprints"
          className="pl-8"
        />
      </div>
      <Select
        value={type ?? 'all'}
        onValueChange={value => {
          onTypeChange(
            value === 'all' ? undefined : (value as Blueprint['type'])
          )
        }}
      >
        <SelectTrigger className="w-[150px]" aria-label="Filter by type">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All types</SelectItem>
          <SelectItem value="general">General</SelectItem>
          <SelectItem value="claude-code">Claude Code</SelectItem>
          <SelectItem value="claude">Claude</SelectItem>
          <SelectItem value="cursor">Cursor</SelectItem>
          <SelectItem value="codex">Codex</SelectItem>
        </SelectContent>
      </Select>

      <MetadataFilter
        value={metadata}
        onChange={onMetadataChange}
        ariaLabel="Filter blueprints by metadata"
        keys={catalog.keys}
        keysLoading={catalog.keysLoading}
        keysError={catalog.keysError}
        onOpenCatalog={catalog.loadKeys}
        activeKey={catalog.activeKey}
        onSelectKey={catalog.selectKey}
        values={catalog.values}
        valuesLoading={catalog.valuesLoading}
        valuesError={catalog.valuesError}
        valuesTruncated={catalog.valuesTruncated}
        valueQuery={catalog.valueQuery}
        onValueQueryChange={catalog.setValueQuery}
      />

      {hasActiveFilters && onClear && (
        <Button variant="outline" onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
