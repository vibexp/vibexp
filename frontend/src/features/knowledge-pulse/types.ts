/**
 * Wire shape of `GET /api/v1/{team_id}/knowledge-pulse`.
 *
 * Hand-written until the `@vibexp/api-client` release generated from this
 * spec change is adopted; then switch to its `components['schemas']` types.
 * Until then, keep this in step with `backend/schemas/knowledge-pulse.yaml`.
 */

export type PulseEventKind = 'read' | 'write' | 'link' | 'feed_post'

export type PulseResourceType =
  'memory' | 'artifact' | 'prompt' | 'blueprint' | 'feed_item'

export interface PulseActor {
  type: 'user' | 'agent'
  id: string
  name: string
  source?: string
}

export interface PulseResourceRef {
  type: PulseResourceType
  id: string
  title?: string
  slug?: string
  project_id?: string
}

export interface PulseEvent {
  id: string
  kind: PulseEventKind
  at: string
  actor: PulseActor
  resource: PulseResourceRef
  detail?: {
    action?: 'create' | 'update'
    relation_type?: string
    to?: PulseResourceRef
  }
}

export interface PulseNode {
  type: PulseResourceType
  id: string
  title: string
  slug?: string
  project_id?: string
  degree: number
}

export interface PulseEdge {
  from: string
  to: string
  relation_type: string
}

export interface PulseSnapshotActor {
  id: string
  name: string
  type: 'user' | 'agent'
}

export interface PulseSnapshot {
  nodes: PulseNode[]
  edges: PulseEdge[]
  actors: PulseSnapshotActor[]
}

export interface PulseStats {
  reads: number
  writes: number
  links: number
  compounding_score: number
}

export interface KnowledgePulseResponse {
  cursor: string
  events: PulseEvent[]
  snapshot?: PulseSnapshot | null
  stats_today: PulseStats
}
