import { Search } from 'lucide-react'

import { ProjectPicker } from '@/components/ProjectPicker'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useTypes } from '@/hooks/useTypes'
import { ARTIFACT_STATUS_OPTIONS } from '@/pages/artifacts/artifactStatus'
import type { Artifact } from '@/types'

interface Props {
  searchInput: string
  onSearchInputChange: (value: string) => void
  projectId: string | undefined
  onProjectChange: (value: string | undefined) => void
  type: Artifact['type'] | undefined
  onTypeChange: (value: Artifact['type'] | undefined) => void
  status: Artifact['status'] | undefined
  onStatusChange: (value: Artifact['status'] | undefined) => void
}

export function ArtifactFilters({
  searchInput,
  onSearchInputChange,
  projectId,
  onProjectChange,
  type,
  onTypeChange,
  status,
  onStatusChange,
}: Props) {
  const { types } = useTypes('artifacts')
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search artifacts…"
          className="pl-8"
        />
      </div>
      <div className="w-[200px]">
        <ProjectPicker
          value={projectId ?? null}
          onChange={value => {
            onProjectChange(value ?? undefined)
          }}
          includeAllOption
          allOptionLabel="All projects"
          data-testid="artifact-project-filter"
        />
      </div>
      <Select
        value={type ?? 'all'}
        onValueChange={value => {
          onTypeChange(value === 'all' ? undefined : value)
        }}
      >
        <SelectTrigger className="w-[150px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All types</SelectItem>
          {types.map(t => (
            <SelectItem key={t.id} value={t.slug}>
              {t.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={status ?? 'all'}
        onValueChange={value => {
          onStatusChange(
            value === 'all' ? undefined : (value as Artifact['status'])
          )
        }}
      >
        <SelectTrigger
          className="w-[150px]"
          data-testid="artifact-status-filter"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All statuses</SelectItem>
          {ARTIFACT_STATUS_OPTIONS.map(option => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
