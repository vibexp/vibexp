import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Blueprint } from '@/services/blueprintService'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const TYPE_LABEL: Record<Blueprint['type'], string> = {
  general: 'General',
  'claude-code': 'Claude Code',
  claude: 'Claude',
  cursor: 'Cursor',
  codex: 'Codex',
}

export function buildBlueprintsColumns({
  navigate,
  onDelete,
  canDelete,
}: {
  navigate: NavigateFunction
  onDelete: (blueprint: Blueprint) => void
  /**
   * Whether the current user may delete this blueprint — own vs any, decided
   * per row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (blueprint: Blueprint) => boolean
}): ColumnDef<Blueprint>[] {
  return [
    {
      accessorKey: 'title',
      header: 'Title',
      cell: ({ row }) => {
        const a = row.original
        const base = `/blueprints/${encodeURIComponent(a.project_id)}/${encodeURIComponent(a.slug)}`
        return (
          <div className="max-w-md space-y-0.5">
            <button
              type="button"
              className="hover:text-primary text-left text-sm font-medium underline-offset-2 hover:underline"
              onClick={() => {
                void navigate(base)
              }}
            >
              {a.title}
            </button>
            {a.description && (
              <p className="text-muted-foreground text-xs">
                {a.description.slice(0, 100)}
                {a.description.length > 100 ? '…' : ''}
              </p>
            )}
          </div>
        )
      },
    },
    {
      accessorKey: 'type',
      header: 'Type',
      cell: ({ row }) => (
        <Badge variant="outline">{TYPE_LABEL[row.original.type]}</Badge>
      ),
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <StatusBadge
          tone={row.original.status === 'active' ? 'success' : 'neutral'}
        >
          {row.original.status}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'updated_at',
      header: 'Updated',
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {formatDate(row.original.updated_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      cell: ({ row }) => {
        const a = row.original
        const base = `/blueprints/${encodeURIComponent(a.project_id)}/${encodeURIComponent(a.slug)}`
        return (
          <div className="flex justify-end gap-1 opacity-60 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
            <Button
              variant="ghost"
              size="icon"
              aria-label="View"
              onClick={() => {
                void navigate(base)
              }}
            >
              <Eye className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Edit"
              onClick={() => {
                void navigate(`${base}/edit`)
              }}
            >
              <Pencil className="size-4" />
            </Button>
            {canDelete(a) && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Delete"
                onClick={() => {
                  onDelete(a)
                }}
              >
                <Trash2 className="size-4" />
              </Button>
            )}
          </div>
        )
      },
    },
  ]
}
