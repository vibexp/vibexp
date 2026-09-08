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
  fieldOfRole,
  type FieldSpec,
  getResourceDescriptor,
} from '@/components/patterns/resource'
import { markdownToExcerpt } from '@/lib/markdownExcerpt'
import { extractTags } from '@/pages/memories/memoryRequest'
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

/**
 * What the primary column shows: the title when the memory has one, and an
 * excerpt of the body otherwise.
 *
 * Blank-checked rather than null-checked. The schema puts no `minLength` on
 * `title` (`backend/schemas/memories.yaml`), so a memory written through the
 * API or MCP can carry `""` — and treating only `null` as absent would render
 * an empty Content cell, which is the #909 failure over again.
 */
function contentCellValue(memory: Memory): string {
  const title = memory.title?.trim() ?? ''
  return title === ''
    ? markdownToExcerpt(memory.text, MEMORY_EXCERPT_LENGTH)
    : title
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
      // The BODY field, not the `name` one, and deliberately: a memory's title
      // is optional (#911) so most rows still show a body excerpt, and the
      // endpoint's `sort_by` accepts `text` and has no `title` value — keying
      // the column on `title` would silently drop its sort header.
      field: fieldOfRole(descriptor, 'body'),
      // Which is also why it reads "Content" rather than the field's label.
      header: 'Content',
      // The title when there is one; otherwise an excerpt of the body — plain
      // text, not markdown, because the raw body puts `#` and `**` in the
      // memory's only identifying cell (#909).
      value: contentCellValue,
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
