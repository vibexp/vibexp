import type {
  PulseEdge,
  PulseNode,
  PulseResourceType,
  PulseSnapshotActor,
} from './types'

/** Order of the type arcs around the ring, clockwise from 12 o'clock. */
export const RING_TYPE_ORDER: readonly PulseResourceType[] = [
  'memory',
  'artifact',
  'prompt',
  'blueprint',
  'feed_item',
]

/** Most nodes the ring draws before it stops being readable. */
export const MAX_RING_NODES = 150

export const VIEWBOX = 920
export const CENTER = VIEWBOX / 2
export const NODE_RING_RADIUS = 285
export const ACTOR_RING_RADIUS = 395

/** Empty slots left between two type arcs so the groups read apart. */
const GROUP_GAP_SLOTS = 2

/** Narrowest a type arc may be (its label needs the room). */
const MIN_ARC_RADIANS = 0.55

export interface PlacedNode extends PulseNode {
  x: number
  y: number
  /** Angle in radians, 0 = 12 o'clock, clockwise. */
  angle: number
  r: number
}

export interface PlacedActor extends PulseSnapshotActor {
  x: number
  y: number
  angle: number
}

export interface TypeArc {
  type: PulseResourceType
  start: number
  end: number
  count: number
}

export interface RingLayout {
  nodes: PlacedNode[]
  byId: Map<string, PlacedNode>
  arcs: TypeArc[]
  actors: PlacedActor[]
  actorById: Map<string, PlacedActor>
}

export function polar(angle: number, radius: number): { x: number; y: number } {
  return {
    x: CENTER + radius * Math.sin(angle),
    y: CENTER - radius * Math.cos(angle),
  }
}

/** Node dot radius: grows with the square root of its link count. */
export function nodeRadius(degree: number): number {
  return Math.min(11, 3.5 + Math.sqrt(Math.max(0, degree)) * 1.6)
}

/**
 * Keep the ring readable: when there are more than `max` nodes, keep the
 * best-connected ones plus anything touched recently (`recent`), dropping the
 * isolated, quiet tail.
 */
export function capNodes(
  nodes: readonly PulseNode[],
  recent: ReadonlySet<string>,
  max = MAX_RING_NODES
): PulseNode[] {
  if (nodes.length <= max) return [...nodes]
  const ranked = [...nodes].sort((a, b) => {
    const ra = recent.has(a.id) ? 1 : 0
    const rb = recent.has(b.id) ? 1 : 0
    return rb - ra || b.degree - a.degree || a.id.localeCompare(b.id)
  })
  return ranked.slice(0, max)
}

/**
 * Place nodes around the ring, grouped into one arc per resource type, and
 * actors evenly on an outer ring. Pure and deterministic: the same input
 * always yields the same positions, so a re-render never shuffles the ring.
 */
export function layoutRing(
  nodes: readonly PulseNode[],
  actors: readonly PulseSnapshotActor[]
): RingLayout {
  const groups = RING_TYPE_ORDER.map(type => ({
    type,
    items: nodes
      .filter(n => n.type === type)
      .sort(
        (a, b) =>
          (a.project_id ?? '').localeCompare(b.project_id ?? '') ||
          a.title.localeCompare(b.title) ||
          a.id.localeCompare(b.id)
      ),
  })).filter(g => g.items.length > 0)

  // Every arc gets at least MIN_ARC_RADIANS so a one-item type still has
  // room for its curved label; the minimum is in slots, which depend on the
  // total, so settle it with a few fixed-point passes.
  const gapSlots = groups.length > 1 ? GROUP_GAP_SLOTS : 0
  let minSlots = 1
  let slots = 1
  for (let pass = 0; pass < 4; pass++) {
    slots = Math.max(
      1,
      groups.reduce(
        (n, g) => n + Math.max(g.items.length, minSlots) + gapSlots,
        0
      )
    )
    minSlots = Math.ceil((MIN_ARC_RADIANS * slots) / (2 * Math.PI))
  }
  const step = (2 * Math.PI) / slots

  const placed: PlacedNode[] = []
  const arcs: TypeArc[] = []
  let slot = 0
  for (const group of groups) {
    const span = Math.max(group.items.length, minSlots)
    const start = slot * step
    // Centre the dots inside an arc that is wider than its items.
    const offset = (span - group.items.length) / 2
    group.items.forEach((node, i) => {
      const angle = (slot + offset + i) * step + step / 2
      placed.push({
        ...node,
        angle,
        r: nodeRadius(node.degree),
        ...polar(angle, NODE_RING_RADIUS),
      })
    })
    arcs.push({
      type: group.type,
      start,
      end: (slot + span) * step,
      count: group.items.length,
    })
    slot += span + gapSlots
  }

  const sortedActors = [...actors].sort(
    (a, b) => a.type.localeCompare(b.type) || a.name.localeCompare(b.name)
  )
  const actorStep = (2 * Math.PI) / Math.max(1, sortedActors.length)
  const placedActors = sortedActors.map((actor, i) => {
    const angle = i * actorStep + actorStep / 2
    return { ...actor, angle, ...polar(angle, ACTOR_RING_RADIUS) }
  })

  return {
    nodes: placed,
    byId: new Map(placed.map(n => [n.id, n])),
    arcs,
    actors: placedActors,
    actorById: new Map(placedActors.map(a => [a.id, a])),
  }
}

/**
 * The curve for a relation: a quadratic Bézier bowed toward the centre, which
 * gives the chord-diagram look. The closer the two ends sit on the ring, the
 * less the curve bows, so neighbours get a short arc, not a loop.
 */
export function edgePath(
  a: { x: number; y: number },
  b: { x: number; y: number }
): string {
  const midX = (a.x + b.x) / 2
  const midY = (a.y + b.y) / 2
  const pull = 0.85
  const cx = midX + (CENTER - midX) * pull
  const cy = midY + (CENTER - midY) * pull
  return `M${fmt(a.x)},${fmt(a.y)} Q${fmt(cx)},${fmt(cy)} ${fmt(b.x)},${fmt(b.y)}`
}

/** SVG arc path along a circle of `radius` from `start` to `end` (radians). */
export function arcPath(start: number, end: number, radius: number): string {
  const a = polar(start, radius)
  const b = polar(end, radius)
  const large = end - start > Math.PI ? 1 : 0
  return `M${fmt(a.x)},${fmt(a.y)} A${String(radius)},${String(radius)} 0 ${String(large)} 1 ${fmt(b.x)},${fmt(b.y)}`
}

/**
 * A path for a label that curves along the ring, centred on `mid`. Labels in
 * the lower half run the other way round so they read left to right instead
 * of upside down. The span is fixed, not the arc's, so a one-item arc still
 * has room for its label.
 */
export function arcLabelPath(
  mid: number,
  radius: number,
  halfSpan = 0.6
): string {
  const lower = Math.cos(mid) < 0
  const a = polar(lower ? mid + halfSpan : mid - halfSpan, radius)
  const b = polar(lower ? mid - halfSpan : mid + halfSpan, radius)
  return `M${fmt(a.x)},${fmt(a.y)} A${String(radius)},${String(radius)} 0 0 ${lower ? '0' : '1'} ${fmt(b.x)},${fmt(b.y)}`
}

/** True when a ring angle sits in the lower half (labels hang below). */
export function isLowerHalf(angle: number): boolean {
  return Math.cos(angle) < 0
}

/** Edges whose both ends are on the ring (the rest are off-screen nodes). */
export function visibleEdges(
  edges: readonly PulseEdge[],
  byId: ReadonlyMap<string, PlacedNode>
): PulseEdge[] {
  return edges.filter(
    e => e.from !== e.to && byId.has(e.from) && byId.has(e.to)
  )
}

function fmt(n: number): string {
  return n.toFixed(1)
}
