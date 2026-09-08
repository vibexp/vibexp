import type { ColumnDef } from '@tanstack/react-table'
import { Eye, FolderOpen, Pencil, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router'

import { FreshnessBadge } from '@/components/FreshnessBadge'
import {
  actionsColumn,
  columnList,
  nameColumn,
  statusColumn,
  taxonomyColumn,
  updatedColumn,
} from '@/components/patterns/list-page'
import {
  type FieldSpec,
  fieldOfRole,
  getResourceDescriptor,
} from '@/components/patterns/resource'
import { markdownToExcerpt } from '@/lib/markdownExcerpt'
import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

const descriptor = getResourceDescriptor('memory')

/** Longest excerpt shown in the list's Content cell. */
const MEMORY_EXCERPT_LENGTH = 140

/**
 * Memory declares no `taxonomy` field: its tags are lifted out of the free-form
 * `metadata` bag, exactly as `ResourceTaxonomySection` does on the detail page
 * (#904). The lift is a display choice, so the spec lives here rather than on
 * the descriptor.
 */
const tagsField: FieldSpec = { key: 'tags', role: 'taxonomy', label: 'Tags' }

export function extractTags(meta?: Record<string, unknown>): string[] {
  const tags = meta?.tags
  if (!Array.isArray(tags)) return []
  return tags.filter((t): t is string => typeof t === 'string')
}

export function buildMemoriesColumns({
  navigate,
  onDelete,
  canDelete,
  includeTags,
  projects = [],
}: {
  navigate: NavigateFunction
  onDelete: (memory: Memory) => void
  /**
   * Whether the current user may delete this memory — own vs any, decided per
   * row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (memory: Memory) => boolean
  includeTags: boolean
  projects?: Project[]
}): ColumnDef<Memory>[] {
  const projectMap = new Map(projects.map(p => [p.id, p]))

  /** Only rendered when the page is not already scoped to one project. */
  const projectColumn: ColumnDef<Memory> = {
    id: 'project',
    header: 'Project',
    cell: ({ row }) => {
      const proj = projectMap.get(row.original.project_id)
      if (!proj) {
        return <span className="text-muted-foreground text-xs">—</span>
      }
      return (
        <span className="flex items-center gap-1 text-xs">
          <FolderOpen className="size-3 shrink-0" />
          {proj.name}
        </span>
      )
    },
  }

  return columnList<Memory>(
    nameColumn<Memory>({
      field: fieldOfRole(descriptor, 'name'),
      // A memory has no title: the list shows an excerpt of its body, which is
      // why this column reads "Content" rather than the descriptor's label.
      header: 'Content',
      // Plain text, not markdown: the raw body puts `#` and `**` in the
      // memory's only identifying cell (#909).
      value: memory => markdownToExcerpt(memory.text, MEMORY_EXCERPT_LENGTH),
      multiline: true,
      className: 'max-w-xl',
      // Renders nothing when the resource is fresh.
      adornment: memory => <FreshnessBadge freshness={memory.freshness} />,
    }),
    projects.length > 0 && projectColumn,
    includeTags &&
      taxonomyColumn<Memory>({
        field: tagsField,
        values: memory => extractTags(memory.metadata),
      }),
    statusColumn<Memory>({
      field: fieldOfRole(descriptor, 'status'),
      value: memory => memory.status,
    }),
    updatedColumn<Memory>({ value: memory => memory.updated_at }),
    actionsColumn<Memory>({
      singular: descriptor.singular,
      actions: [
        {
          key: 'view',
          label: 'View',
          icon: Eye,
          onSelect: memory => {
            void navigate(`/memories/${memory.id}`)
          },
        },
        {
          key: 'edit',
          label: 'Edit',
          icon: Pencil,
          onSelect: memory => {
            void navigate(`/memories/${memory.id}/edit`)
          },
        },
        {
          key: 'delete',
          label: 'Delete',
          icon: Trash2,
          onSelect: onDelete,
          visible: canDelete,
        },
      ],
    })
  )
}
