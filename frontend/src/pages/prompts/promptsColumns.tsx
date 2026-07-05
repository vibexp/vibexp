import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Share2, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import type { Prompt } from '@/services/promptService'

function absTime(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function StatusDotPill({ status }: { status: Prompt['status'] }) {
  const dotClass = status === 'published' ? 'bg-success' : 'bg-warning'
  return (
    <span
      className="bg-muted text-foreground inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium"
      aria-label={`Status: ${status}`}
    >
      <span className={cn('size-1.5 rounded-full', dotClass)} aria-hidden />
      {status}
    </span>
  )
}

export function buildPromptsColumns({
  navigate,
  onDelete,
}: {
  navigate: NavigateFunction
  onDelete: (prompt: Prompt) => void
}): ColumnDef<Prompt>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <div className="min-w-0 max-w-md space-y-0.5">
          <button
            type="button"
            className="hover:text-primary block w-full truncate text-left text-sm font-medium underline-offset-2 hover:underline"
            onClick={() => {
              void navigate(`/prompts/${row.original.slug}`)
            }}
          >
            {row.original.name}
          </button>
          {row.original.description && (
            <p className="text-muted-foreground truncate text-xs">
              {row.original.description}
            </p>
          )}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => <StatusDotPill status={row.original.status} />,
    },
    {
      id: 'shared',
      header: 'Shared',
      cell: ({ row }) =>
        row.original.is_shared ? (
          <span className="text-muted-foreground inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs">
            <Share2 className="size-3" />
            Shared
          </span>
        ) : (
          <span className="text-muted-foreground text-xs">—</span>
        ),
    },
    {
      id: 'labels',
      header: 'Labels',
      cell: ({ row }) => {
        const labels = row.original.labels ?? []
        if (labels.length === 0) {
          return <span className="text-muted-foreground text-xs">—</span>
        }
        return (
          <div className="flex flex-wrap gap-1">
            {labels.slice(0, 3).map(label => (
              <Badge
                key={label}
                variant="outline"
                className="bg-muted text-foreground rounded px-1.5 py-0 font-mono text-xs font-medium tracking-tight"
              >
                {label}
              </Badge>
            ))}
            {labels.length > 3 && (
              <Badge
                variant="outline"
                className="bg-muted text-foreground rounded px-1.5 py-0 font-mono text-xs font-medium tracking-tight"
              >
                +{labels.length - 3}
              </Badge>
            )}
          </div>
        )
      },
    },
    {
      accessorKey: 'updated_at',
      header: 'Updated',
      cell: ({ row }) => (
        <span
          className="text-muted-foreground whitespace-nowrap text-xs tabular-nums"
          title={absTime(row.original.updated_at)}
        >
          {formatRelativeTime(row.original.updated_at)}
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
              void navigate(`/prompts/${row.original.slug}`)
            }}
          >
            <Eye className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Edit"
            onClick={() => {
              // Editor still lives in v1 until Slice 5b lands
              void navigate(`/prompts/${row.original.slug}/edit`)
            }}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Delete"
            data-testid="delete-prompt-button"
            onClick={() => {
              onDelete(row.original)
            }}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ),
    },
  ]
}
