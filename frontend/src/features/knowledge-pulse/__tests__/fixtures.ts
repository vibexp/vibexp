import type {
  KnowledgePulseResponse,
  PulseEvent,
  PulseNode,
  PulseSnapshot,
} from '../types'

export const node = (
  id: string,
  type: PulseNode['type'],
  degree = 0,
  extra: Partial<PulseNode> = {}
): PulseNode => ({ id, type, title: `Title ${id}`, degree, ...extra })

export const pulseEvent = (
  over: Partial<PulseEvent> & Pick<PulseEvent, 'id'>
): PulseEvent => ({
  kind: 'read',
  at: new Date().toISOString(),
  actor: {
    type: 'agent',
    id: 'agent:claude-code',
    name: 'Claude Code',
    source: 'mcp',
  },
  resource: { type: 'memory', id: 'm1', title: 'Deploy runbook' },
  ...over,
})

export const snapshot = (): PulseSnapshot => ({
  nodes: [
    node('m1', 'memory', 2, { title: 'Deploy runbook' }),
    node('m2', 'memory', 1, { title: 'Retry policy' }),
    node('a1', 'artifact', 1, {
      title: 'Incident report',
      slug: 'incident-report',
      project_id: 'p1',
    }),
    node('p1', 'prompt', 0, { title: 'Code review', slug: 'code-review' }),
  ],
  edges: [
    { from: 'm1', to: 'a1', relation_type: 'built-from' },
    { from: 'm1', to: 'm2', relation_type: 'supersedes' },
  ],
  actors: [
    { id: 'agent:claude-code', name: 'Claude Code', type: 'agent' },
    { id: 'u1', name: 'Ada Lovelace', type: 'user' },
  ],
})

export const response = (
  over: Partial<KnowledgePulseResponse> = {}
): KnowledgePulseResponse => ({
  cursor: '2026-10-19T10:00:00Z',
  events: [],
  snapshot: snapshot(),
  stats_today: { reads: 500, writes: 40, links: 9, compounding_score: 14 },
  ...over,
})
