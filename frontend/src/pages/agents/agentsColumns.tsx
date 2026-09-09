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
import {
  fieldOfRole,
  getResourceDescriptor,
} from '@/components/patterns/resource'
import type { Agent } from '@/services/agentService'

import { agentStatusField } from './agentStatus'
import { successRateColor } from './helpers'

/**
 * An agent has been a registered, descriptor-only kind since #918, so the list
 * reads its field specs off the descriptor rather than restating them: the same
 * name and status specs drive the agents list and the agent detail page, which
 * is what keeps one status badge, one label and one set of action test ids
 * across both. Status still arrives through `./agentStatus`, the module the
 * descriptor re-exports it from.
 */
const AGENT = getResourceDescriptor('agent')
const nameField = fieldOfRole(AGENT, 'name')

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
    statusColumn<Agent>({
      field: agentStatusField,
      value: agent => agent.status,
    }),
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
