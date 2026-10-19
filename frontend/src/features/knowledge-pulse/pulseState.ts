import type {
  PulseEdge,
  PulseEvent,
  PulseNode,
  PulseResourceRef,
  PulseSnapshot,
  PulseSnapshotActor,
  PulseStats,
} from './types'

/** How long one pulse (node halo + actor line) stays on screen. */
export const PULSE_DURATION_MS = 1600
/** Rows kept in the live event feed. */
export const FEED_LIMIT = 60
/** Stats samples kept for the in-session sparklines. */
export const HISTORY_LIMIT = 90

export interface ActivePulse {
  key: string
  nodeId: string
  /** Second end of a link event, so both ends light up. */
  toId?: string
  actorId: string
  kind: PulseEvent['kind']
  startedAt: number
}

export interface StatsSample {
  t: number
  reads: number
  writes: number
  links: number
  compounding: number
}

export interface PulseState {
  nodes: Map<string, PulseNode>
  edges: PulseEdge[]
  actors: Map<string, PulseSnapshotActor>
  feed: PulseEvent[]
  pulses: ActivePulse[]
  /** Touches per node since the page opened (the "top items" panel). */
  touches: Map<string, number>
  lastActivity: Map<string, string>
  stats: PulseStats | null
  history: StatsSample[]
  /** Event ids already applied, so an overlapping poll never double-counts. */
  seen: Set<string>
}

export type PulseAction =
  | { type: 'snapshot'; snapshot: PulseSnapshot }
  | { type: 'stats'; stats: PulseStats; at: number }
  /** `animate: false` applies the event silently (initial backfill). */
  | { type: 'event'; event: PulseEvent; now: number; animate: boolean }
  | { type: 'prune'; now: number }
  | { type: 'reset' }

export function initialPulseState(): PulseState {
  return {
    nodes: new Map(),
    edges: [],
    actors: new Map(),
    feed: [],
    pulses: [],
    touches: new Map(),
    lastActivity: new Map(),
    stats: null,
    history: [],
    seen: new Set(),
  }
}

function edgeKey(e: Pick<PulseEdge, 'from' | 'to' | 'relation_type'>): string {
  return `${e.from}|${e.to}|${e.relation_type}`
}

function ensureNode(
  nodes: Map<string, PulseNode>,
  ref: PulseResourceRef
): void {
  const existing = nodes.get(ref.id)
  if (existing) {
    // A live event can carry a fresher title than the snapshot (a rename).
    if (ref.title && ref.title !== existing.title) {
      nodes.set(ref.id, { ...existing, title: ref.title })
    }
    return
  }
  nodes.set(ref.id, {
    type: ref.type,
    id: ref.id,
    title: ref.title ?? 'Untitled',
    slug: ref.slug,
    project_id: ref.project_id,
    degree: 0,
  })
}

function applyEvent(
  state: PulseState,
  event: PulseEvent,
  now: number,
  animate: boolean
): PulseState {
  if (state.seen.has(event.id)) return state

  const nodes = new Map(state.nodes)
  ensureNode(nodes, event.resource)

  let edges = state.edges
  const to = event.detail?.to
  if (event.kind === 'link' && to) {
    ensureNode(nodes, to)
    const edge: PulseEdge = {
      from: event.resource.id,
      to: to.id,
      relation_type: event.detail?.relation_type ?? 'related',
    }
    const known = new Set(edges.map(edgeKey))
    if (!known.has(edgeKey(edge))) {
      edges = [...edges, edge]
      for (const id of [edge.from, edge.to]) {
        const n = nodes.get(id)
        if (n) nodes.set(id, { ...n, degree: n.degree + 1 })
      }
    }
  }

  const actors = state.actors.has(event.actor.id)
    ? state.actors
    : new Map(state.actors).set(event.actor.id, {
        id: event.actor.id,
        name: event.actor.name,
        type: event.actor.type,
      })

  const touches = new Map(state.touches)
  touches.set(event.resource.id, (touches.get(event.resource.id) ?? 0) + 1)
  const lastActivity = new Map(state.lastActivity)
  lastActivity.set(event.resource.id, event.at)
  if (to) lastActivity.set(to.id, event.at)

  const seen = new Set(state.seen).add(event.id)
  const feed = [event, ...state.feed].slice(0, FEED_LIMIT)
  const pulses = animate
    ? [
        ...state.pulses,
        {
          key: event.id,
          nodeId: event.resource.id,
          toId: event.kind === 'link' ? to?.id : undefined,
          actorId: event.actor.id,
          kind: event.kind,
          startedAt: now,
        },
      ]
    : state.pulses

  return {
    ...state,
    nodes,
    edges,
    actors,
    feed,
    pulses,
    touches,
    lastActivity,
    seen,
  }
}

export function pulseReducer(
  state: PulseState,
  action: PulseAction
): PulseState {
  switch (action.type) {
    case 'snapshot': {
      // The snapshot is authoritative for structure, but nodes and actors
      // discovered from live events since then are kept until it catches up.
      const nodes = new Map(action.snapshot.nodes.map(n => [n.id, n]))
      for (const [id, n] of state.nodes) if (!nodes.has(id)) nodes.set(id, n)
      const actors = new Map(state.actors)
      for (const a of action.snapshot.actors) actors.set(a.id, a)
      return { ...state, nodes, edges: action.snapshot.edges, actors }
    }
    case 'stats': {
      const sample: StatsSample = {
        t: action.at,
        reads: action.stats.reads,
        writes: action.stats.writes,
        links: action.stats.links,
        compounding: action.stats.compounding_score,
      }
      return {
        ...state,
        stats: action.stats,
        history: [...state.history, sample].slice(-HISTORY_LIMIT),
      }
    }
    case 'event':
      return applyEvent(state, action.event, action.now, action.animate)
    case 'prune': {
      const pulses = state.pulses.filter(
        p => action.now - p.startedAt < PULSE_DURATION_MS
      )
      return pulses.length === state.pulses.length
        ? state
        : { ...state, pulses }
    }
    case 'reset':
      return initialPulseState()
  }
}

/**
 * Delays (ms) that spread `count` events evenly across one poll interval, so
 * a burst that arrived in a single poll still plays out as steady motion.
 */
export function spreadDelays(count: number, intervalMs: number): number[] {
  if (count <= 0) return []
  const step = intervalMs / count
  return Array.from({ length: count }, (_, i) => Math.round(i * step))
}

/** Oldest first, so the replay runs in the order things happened. */
export function sortEventsByTime(events: readonly PulseEvent[]): PulseEvent[] {
  return [...events].sort(
    (a, b) => Date.parse(a.at) - Date.parse(b.at) || a.id.localeCompare(b.id)
  )
}
