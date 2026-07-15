import type { ColumnDef } from '@tanstack/react-table'
import { Eye, FolderOpen, Pencil, Tag as TagIcon, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  MEMORY_STATUS_LABEL,
  memoryStatusTone,
} from '@/pages/memories/memoryStatus'
import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function extractTags(meta?: Record<string, unknown>): string[] {
  const tags = meta?.tags
  if (!Array.isArray(tags)) return []
  return tags.filter((t): t is string => typeof t === 'string')
}

function truncate(text: string, max = 140) {
  if (text.length <= max) return text
  return text.slice(0, max) + '…'
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

  const columns: ColumnDef<Memory>[] = [
    {
      accessorKey: 'text',
      header: 'Content',
      cell: ({ row }) => (
        <div className="max-w-xl space-y-1">
          <p className="text-sm leading-relaxed">
            {truncate(row.original.text)}
          </p>
        </div>
      ),
    },
  ]

  if (projects.length > 0) {
    columns.push({
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
    })
  }

  if (includeTags) {
    columns.push({
      id: 'tags',
      header: 'Tags',
      cell: ({ row }) => {
        const tags = extractTags(row.original.metadata)
        if (tags.length === 0) {
          return <span className="text-muted-foreground text-xs">—</span>
        }
        return (
          <div className="flex flex-wrap gap-1">
            {tags.slice(0, 3).map(tag => (
              <Badge key={tag} variant="secondary" className="gap-1">
                <TagIcon className="size-3" />
                {tag}
              </Badge>
            ))}
            {tags.length > 3 && (
              <Badge variant="outline">+{tags.length - 3}</Badge>
            )}
          </div>
        )
      },
    })
  }

  columns.push(
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <StatusBadge tone={memoryStatusTone(row.original.status)}>
          {MEMORY_STATUS_LABEL[row.original.status]}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'updated_at',
      header: 'Updated',
      cell: ({ row }) => (
        <span className="text-muted-foreground whitespace-nowrap text-xs tabular-nums">
          {formatDate(row.original.updated_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <div className="flex justify-end gap-1 opacity-60 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
          <Button
            variant="ghost"
            size="icon"
            aria-label="View"
            onClick={() => {
              void navigate(`/memories/${row.original.id}`)
            }}
          >
            <Eye className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Edit"
            onClick={() => {
              void navigate(`/memories/${row.original.id}/edit`)
            }}
          >
            <Pencil className="size-4" />
          </Button>
          {canDelete(row.original) && (
            <Button
              variant="ghost"
              size="icon"
              aria-label="Delete"
              onClick={() => {
                onDelete(row.original)
              }}
            >
              <Trash2 className="size-4" />
            </Button>
          )}
        </div>
      ),
    }
  )

  return columns
}
