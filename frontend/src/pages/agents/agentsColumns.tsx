import type { ColumnDef } from '@tanstack/react-table'
import { Edit, MessageSquare, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Agent } from '@/types'

import {
  agentStatusLabel,
  agentStatusVariant,
  formatDate,
  successRateColor,
} from './helpers'

export function buildAgentsColumns({
  navigate,
  onDelete,
}: {
  navigate: NavigateFunction
  onDelete: (agent: Agent) => void
}): ColumnDef<Agent>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <div>
          <div className="text-primary font-medium hover:underline">
            {row.original.name}
          </div>
          <div className="text-muted-foreground line-clamp-1 max-w-xs text-xs">
            {row.original.description || 'No description'}
          </div>
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <Badge variant={agentStatusVariant(row.original.status)}>
          {agentStatusLabel(row.original.status)}
        </Badge>
      ),
    },
    {
      accessorKey: 'total_runs',
      header: 'Total runs',
      meta: { align: 'right' },
      cell: ({ row }) => (
        <span className="font-mono text-sm">
          {row.original.total_runs.toLocaleString()}
        </span>
      ),
    },
    {
      accessorKey: 'success_rate',
      header: 'Success rate',
      meta: { align: 'right' },
      cell: ({ row }) => {
        const percentage = Math.round(row.original.success_rate)
        return (
          <span className={`font-medium ${successRateColor(percentage)}`}>
            {percentage}%
          </span>
        )
      },
    },
    {
      accessorKey: 'last_run',
      header: 'Last run',
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {formatDate(row.original.last_run)}
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
            aria-label={`Chat with ${row.original.name}`}
            onClick={() => {
              void navigate(`/agents/${row.original.id}/chat`)
            }}
          >
            <MessageSquare className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Edit ${row.original.name}`}
            onClick={() => {
              void navigate(`/agents/${row.original.id}/edit`)
            }}
          >
            <Edit className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Delete ${row.original.name}`}
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
