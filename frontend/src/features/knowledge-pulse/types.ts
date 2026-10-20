import type { components } from '@vibexp/api-client'

/**
 * The `GET /api/v1/{team_id}/knowledge-pulse` wire shapes, under the short
 * names the feature uses. All generated from `backend/schemas/knowledge-pulse.yaml`.
 */
type Schemas = components['schemas']

export type PulseEventKind = Schemas['KnowledgePulseEventKind']
export type PulseResourceType = Schemas['KnowledgePulseResourceType']
export type PulseActor = Schemas['KnowledgePulseActor']
export type PulseEvent = Schemas['KnowledgePulseEvent']
export type PulseNode = Schemas['KnowledgePulseNode']
export type PulseEdge = Schemas['KnowledgePulseEdge']
export type PulseSnapshotActor = Schemas['KnowledgePulseSnapshotActor']
export type PulseSnapshot = Schemas['KnowledgePulseSnapshot']
export type PulseStats = Schemas['KnowledgePulseStats']
export type KnowledgePulseResponse = Schemas['KnowledgePulseResponse']

/**
 * A resource reference as the feature handles it: an event's own resource
 * (title always set) or a link's far end (title optional).
 */
export type PulseResourceRef =
  | Schemas['KnowledgePulseResourceRef']
  | (Schemas['KnowledgePulseRelatedRef'] & { project_id?: string })
