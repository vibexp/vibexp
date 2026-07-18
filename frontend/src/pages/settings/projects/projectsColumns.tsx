import type { ColumnDef } from '@tanstack/react-table'
import {
  ArrowRightLeft,
  ExternalLink,
  GitBranch,
  Globe,
  Pencil,
  Trash2,
} from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { Project } from '@/services/projectService'

function formatDate(value: string) {
  return new Date(value).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function ExternalLinkCell({
  url,
  icon: Icon,
}: Readonly<{
  url: string | undefined
  icon: typeof GitBranch
}>) {
  if (!url) {
    return <span className="text-muted-foreground text-xs">—</span>
  }
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
      onClick={e => {
        e.stopPropagation()
      }}
    >
      <Icon className="size-4" />
      <ExternalLink className="size-3" />
    </a>
  )
}

export function buildProjectsColumns({
  navigate,
  onDelete,
  canUpdate,
  canDelete,
}: {
  navigate: NavigateFunction
  onDelete: (project: Project) => void
  /** `project.update` — gates Edit and Migrate resources (#225). */
  canUpdate: boolean
  /** `project.delete` — gates Delete (#225). */
  canDelete: boolean
}): ColumnDef<Project>[] {
  const columns: ColumnDef<Project>[] = [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <div className="space-y-0.5">
          <button
            type="button"
            className="hover:text-primary text-left text-sm font-medium underline-offset-2 hover:underline"
            onClick={() => {
              void navigate(
                `/settings/projects/${encodeURIComponent(row.original.slug)}`
              )
            }}
          >
            {row.original.name}
          </button>
          {row.original.description && (
            <p className="text-muted-foreground text-xs">
              {row.original.description.slice(0, 100)}
              {row.original.description.length > 100 ? '…' : ''}
            </p>
          )}
        </div>
      ),
    },
    {
      accessorKey: 'slug',
      header: 'Slug',
      cell: ({ row }) => (
        <code className="bg-muted rounded px-2 py-0.5 font-mono text-xs">
          {row.original.slug}
        </code>
      ),
    },
    {
      id: 'git_url',
      header: 'Git',
      cell: ({ row }) => (
        <ExternalLinkCell url={row.original.git_url} icon={GitBranch} />
      ),
    },
    {
      id: 'homepage',
      header: 'Homepage',
      cell: ({ row }) => (
        <ExternalLinkCell url={row.original.homepage} icon={Globe} />
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
      cell: ({ row }) => (
        <TooltipProvider>
          <div className="flex justify-end gap-1 opacity-60 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
            {canUpdate && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Edit"
                onClick={() => {
                  void navigate(
                    `/settings/projects/edit/${encodeURIComponent(row.original.slug)}`
                  )
                }}
              >
                <Pencil className="size-4" />
              </Button>
            )}
            {canUpdate && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Migrate resources"
                    onClick={() => {
                      void navigate(
                        `/settings/projects/${encodeURIComponent(row.original.slug)}/migrate`
                      )
                    }}
                  >
                    <ArrowRightLeft className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Migrate resources</TooltipContent>
              </Tooltip>
            )}
            {canDelete && (
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
        </TooltipProvider>
      ),
    },
  ]

  // Members hold no project permissions at all (epic #220 decision D2), so drop
  // the column rather than leave an empty one hanging off every row.
  return canUpdate || canDelete
    ? columns
    : columns.filter(column => column.id !== 'actions')
}
