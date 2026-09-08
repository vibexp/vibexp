import { formatDate } from '@/lib/time'

import { defineResource } from '../defineResource'
import type { FieldSpec } from '../types'

/**
 * The agent's status field, exported on its own because the agents list column
 * (`pages/agents/agentsColumns.tsx`) reads a `FieldSpec` directly rather than a
 * whole descriptor. It is the same object the descriptor carries, so there is
 * still exactly one tone/label table per enum (#907).
 */
export const agentStatusField: FieldSpec = {
  key: 'status',
  role: 'status',
  label: 'Status',
  statusValues: ['active', 'paused', 'error'],
  tone: { active: 'success', paused: 'neutral', error: 'destructive' },
  valueLabels: { active: 'Active', paused: 'Paused', error: 'Error' },
}

/**
 * Agent — a descriptor-only kind (#918), like the gallery prompt.
 *
 * An agent is not a team *resource*: it is served by the agents API, has no
 * team-scoped resource id, and none of the shared side panels (attachments,
 * comments, relations, versions) has an agent endpoint. So it is absent from
 * `ResourceKind` and its detail page passes no `resource` — every standard
 * panel drops out on its own. Access activity is the exception and arrives as
 * an `extraSections` entry, because `ResourceAccessType` does include `agent`.
 *
 * Unlike the gallery prompt it is NOT `readOnly`: an agent is created, edited
 * and deleted in the app. It simply has no `form` spec yet — the agent editor
 * is still hand-written (out of scope for #918).
 */
export const agentDescriptor = defineResource({
  kind: 'agent',
  singular: 'agent',
  plural: 'agents',
  address: ['id'],
  fields: [
    { key: 'name', role: 'name', label: 'Name' },
    { key: 'id', role: 'address', label: 'ID' },
    { key: 'description', role: 'summary', label: 'Description' },
    agentStatusField,
    // The agent card's own version string — distinct from `Agent.version`,
    // which is the optimistic-locking counter and means nothing to a reader.
    {
      key: 'agent_card.version',
      role: 'meta',
      label: 'Card version',
      optional: true,
    },
    {
      key: 'last_synced_at',
      role: 'meta',
      label: 'Last synced',
      optional: true,
      // A raw ISO timestamp beside the relative Created / Updated rows reads
      // as a different kind of value; format it the way the page always has.
      render: value => (typeof value === 'string' ? formatDate(value) : null),
    },
  ],
  capabilities: {
    attachments: false,
    versions: false,
    comments: false,
    relations: false,
    mcp: false,
  },
} as const)
