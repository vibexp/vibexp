import type { ColumnDef } from '@tanstack/react-table'
import { Edit, MessageSquare, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router'

import {
  actionsColumn,
  columnList,
  nameColumn,
  statusColumn,
  updatedColumn,
} from '@/components/patterns/list-page'
import type { FieldSpec } from '@/components/patterns/resource'
import type { Agent } from '@/services/agentService'

import { successRateColor } from './helpers'

/**
 * An agent is not a team resource, so it has no entry in the descriptor
 * registry and nothing to look a `FieldSpec` up on. Declaring the two the list
 * needs here keeps the agents list on the same factories — and therefore on the
 * same status badge, relative timestamps and action test ids — as the four
 * resource lists, without inventing a registry entry for a kind that has no
 * detail route, capabilities or address shape.
 */
const nameField: FieldSpec = { key: 'name', role: 'name', label: 'Name' }
const statusField: FieldSpec = {
  key: 'status',
  role: 'status',
  label: 'Status',
  statusValues: ['active', 'paused', 'error'],
  tone: { active: 'success', paused: 'neutral', error: 'destructive' },
  valueLabels: { active: 'Active', paused: 'Paused', error: 'Error' },
}

export function buildAgentsColumns({
  navigate,
  onDelete,
  canDelete,
}: {
  navigate: NavigateFunction
  onDelete: (agent: Agent) => void
  /**
   * Whether the current user may delete this agent — own vs any, decided per
   * row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (agent: Agent) => boolean
}): ColumnDef<Agent>[] {
  return columnList<Agent>(
    nameColumn<Agent>({
      field: nameField,
      value: agent => agent.name,
      summary: agent => agent.description || 'No description',
      className: 'max-w-xs',
    }),
    statusColumn<Agent>({ field: statusField, value: agent => agent.status }),
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
    updatedColumn<Agent>({
      accessorKey: 'last_run',
      header: 'Last run',
      value: agent => agent.last_run,
    }),
    actionsColumn<Agent>({
      singular: 'agent',
      actions: [
        {
          key: 'chat',
          label: agent => `Chat with ${agent.name}`,
          icon: MessageSquare,
          onSelect: agent => {
            void navigate(`/agents/${agent.id}/chat`)
          },
        },
        {
          key: 'edit',
          label: agent => `Edit ${agent.name}`,
          icon: Edit,
          onSelect: agent => {
            void navigate(`/agents/${agent.id}/edit`)
          },
        },
        {
          key: 'delete',
          label: agent => `Delete ${agent.name}`,
          icon: Trash2,
          onSelect: onDelete,
          visible: canDelete,
        },
      ],
    })
  )
}
