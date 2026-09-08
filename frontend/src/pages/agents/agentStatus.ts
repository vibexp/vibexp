import type { FieldSpec } from '@/components/patterns/resource'

/**
 * Agent status, described the way a resource descriptor describes one.
 *
 * An agent is not a team resource — it has no detail route shape, capabilities
 * or address — so it has no entry in the descriptor registry to look a
 * `FieldSpec` up on. Declaring just the status field here gives the agents list
 * and the agent detail page the same single tone/label table that the four
 * registered kinds get from their descriptors (#903), which is the whole point
 * of #907: one status treatment per enum, not one per page.
 */
export const agentStatusField: FieldSpec = {
  key: 'status',
  role: 'status',
  label: 'Status',
  statusValues: ['active', 'paused', 'error'],
  tone: { active: 'success', paused: 'neutral', error: 'destructive' },
  valueLabels: { active: 'Active', paused: 'Paused', error: 'Error' },
}
