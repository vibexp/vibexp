import './knowledge-pulse.css'

import { type CSSProperties, useEffect, useMemo, useState } from 'react'

import { cn } from '@/lib/utils'

import { AnimatedNumber } from './AnimatedNumber'
import {
  ACTOR_RING_RADIUS,
  arcLabelPath,
  arcPath,
  CENTER,
  edgePath,
  isLowerHalf,
  layoutRing,
  NODE_RING_RADIUS,
  type PlacedActor,
  type PlacedNode,
  VIEWBOX,
  visibleEdges,
} from './layout'
import { type ActivePulse, PULSE_DURATION_MS } from './pulseState'
import {
  EVENT_KIND_COLOR,
  RESOURCE_TYPE_COLOR,
  RESOURCE_TYPE_LABEL,
} from './pulseTheme'
import type { PulseEdge, PulseNode, PulseSnapshotActor } from './types'

interface PulseRingProps {
  nodes: readonly PulseNode[]
  edges: readonly PulseEdge[]
  actors: readonly PulseSnapshotActor[]
  pulses: readonly ActivePulse[]
  lastActivity: ReadonlyMap<string, string>
  reducedMotion: boolean
  /** Polling is running: the centre breathes. */
  live: boolean
  /** How long the ring's counters take to reach a new value. */
  tweenMs: number
  onOpen: (node: PulseNode) => void
  className?: string
}

/** First-load bloom: each node waits this long after the previous one. */
const BLOOM_STAGGER_MS = 6
const BLOOM_SETTLE_MS = 1500

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  const letters =
    parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)
  return letters.toUpperCase()
}

/**
 * The radial knowledge ring: resources around a circle grouped into one arc
 * per type, relations as curves through the middle, people and agents on an
 * outer ring. Each live event pulses its node in the event's colour and sends
 * a beam from the actor who caused it. Nothing snaps: nodes bloom in and glide
 * when the layout shifts, new links draw themselves, counters count.
 */
export function PulseRing({
  nodes,
  edges,
  actors,
  pulses,
  lastActivity,
  reducedMotion,
  live,
  tweenMs,
  onOpen,
  className,
}: Readonly<PulseRingProps>) {
  const layout = useMemo(() => layoutRing(nodes, actors), [nodes, actors])
  const shownEdges = useMemo(
    () => visibleEdges(edges, layout.byId),
    [edges, layout.byId]
  )
  const [hovered, setHovered] = useState<PlacedNode | null>(null)

  // Stagger the bloom only for the first paint; later arrivals bloom at once.
  const [settled, setSettled] = useState(false)
  const hasNodes = layout.nodes.length > 0
  useEffect(() => {
    if (!hasNodes || settled) return
    const id = setTimeout(() => {
      setSettled(true)
    }, BLOOM_SETTLE_MS)
    return () => {
      clearTimeout(id)
    }
  }, [hasNodes, settled])

  const hotNodes = useMemo(() => {
    // Latest pulse per node; its key restarts the halo on a repeat touch.
    const m = new Map<string, { kind: ActivePulse['kind']; key: string }>()
    for (const p of pulses) {
      m.set(p.nodeId, { kind: p.kind, key: p.key })
      if (p.toId) m.set(p.toId, { kind: p.kind, key: p.key })
    }
    return m
  }, [pulses])

  const activeActors = useMemo(
    () => new Set(pulses.map(p => p.actorId)),
    [pulses]
  )

  const hoveredNeighbours = useMemo(() => {
    if (!hovered) return null
    const s = new Set<string>([hovered.id])
    for (const e of shownEdges) {
      if (e.from === hovered.id) s.add(e.to)
      if (e.to === hovered.id) s.add(e.from)
    }
    return s
  }, [hovered, shownEdges])

  if (!hasNodes) {
    return (
      <div
        className={cn(
          'text-muted-foreground flex aspect-square items-center justify-center rounded-full border border-dashed text-sm',
          className
        )}
      >
        No knowledge yet. Activity appears here as people and agents use it.
      </div>
    )
  }

  const motion = {
    '--kp-duration': `${String(PULSE_DURATION_MS)}ms`,
  } as CSSProperties
  const bloomStagger =
    Math.min(BLOOM_STAGGER_MS, BLOOM_SETTLE_MS / 2 / layout.nodes.length) || 0

  return (
    <div className={cn('relative', className)}>
      <svg
        viewBox={`0 0 ${String(VIEWBOX)} ${String(VIEWBOX)}`}
        className="h-auto w-full select-none"
        style={motion}
        role="img"
        aria-label={`Knowledge ring: ${String(layout.nodes.length)} items, ${String(shownEdges.length)} links, ${String(layout.actors.length)} people and agents`}
      >
        <defs>
          <radialGradient id="kp-core">
            <stop
              offset="0%"
              stopColor="var(--foreground)"
              stopOpacity={0.08}
            />
            <stop offset="100%" stopColor="var(--foreground)" stopOpacity={0} />
          </radialGradient>
        </defs>

        {/* Ambient guides: the actor orbit and a soft core. */}
        <circle
          cx={CENTER}
          cy={CENTER}
          r={ACTOR_RING_RADIUS}
          fill="none"
          stroke="var(--border)"
          strokeDasharray="2 6"
        />
        <circle
          cx={CENTER}
          cy={CENTER}
          r={NODE_RING_RADIUS - 30}
          fill="url(#kp-core)"
        >
          {live && !reducedMotion && (
            <animate
              attributeName="opacity"
              values="0.5;1;0.5"
              dur="4s"
              begin="0s"
              repeatCount="indefinite"
            />
          )}
        </circle>

        {/* Type arcs with their labels, just outside the node ring. */}
        {layout.arcs.map(arc => {
          const mid = (arc.start + arc.end) / 2
          const lower = isLowerHalf(mid)
          const labelId = `kp-arc-label-${arc.type}`
          return (
            <g key={arc.type}>
              <path
                d={arcPath(arc.start, arc.end, NODE_RING_RADIUS + 22)}
                fill="none"
                stroke={RESOURCE_TYPE_COLOR[arc.type]}
                strokeWidth={4}
                strokeLinecap="round"
                opacity={0.75}
                className="kp-edge"
              />
              <path
                id={labelId}
                d={arcLabelPath(mid, NODE_RING_RADIUS + (lower ? 34 : 36))}
                fill="none"
              />
              <text
                dominantBaseline={lower ? 'hanging' : 'auto'}
                className="fill-muted-foreground text-[14px] font-semibold tracking-wide uppercase"
              >
                <textPath
                  href={`#${labelId}`}
                  startOffset="50%"
                  textAnchor="middle"
                >
                  {RESOURCE_TYPE_LABEL[arc.type]}{' '}
                  <tspan className="fill-foreground tabular-nums">
                    <AnimatedNumber
                      value={arc.count}
                      durationMs={1200}
                      disabled={reducedMotion}
                    />
                  </tspan>
                </textPath>
              </text>
            </g>
          )
        })}

        {/* Relations. Keyed, so the draw-in runs only when an edge first mounts. */}
        <g fill="none">
          {shownEdges.map(e => {
            const a = layout.byId.get(e.from)
            const b = layout.byId.get(e.to)
            if (!a || !b) return null
            const lit =
              hoveredNeighbours?.has(e.from) && hoveredNeighbours.has(e.to)
            const hot =
              hotNodes.get(e.from)?.kind === 'link' &&
              hotNodes.get(e.to)?.kind === 'link'
            let opacity = 0.3
            if (hoveredNeighbours) opacity = lit ? 0.95 : 0.05
            else if (hot) opacity = 1
            return (
              <path
                key={`${e.from}-${e.to}-${e.relation_type}`}
                d={edgePath(a, b)}
                pathLength={1}
                stroke={
                  hot ? EVENT_KIND_COLOR.link : RESOURCE_TYPE_COLOR[a.type]
                }
                strokeWidth={hot || lit ? 2.25 : 1}
                opacity={opacity}
                className="kp-edge kp-edge-new"
              />
            )
          })}
        </g>

        {/* Beams from the actor to what they touched. */}
        <g strokeLinecap="round">
          {pulses.map(p => {
            const actor = layout.actorById.get(p.actorId)
            const node = layout.byId.get(p.nodeId)
            if (!actor || !node) return null
            return (
              <line
                key={`beam-${p.key}`}
                x1={actor.x}
                y1={actor.y}
                x2={node.x}
                y2={node.y}
                pathLength={1}
                stroke={EVENT_KIND_COLOR[p.kind]}
                strokeWidth={3}
                className="kp-beam"
              />
            )
          })}
        </g>

        {/* Resource nodes. */}
        <g>
          {layout.nodes.map((n, i) => (
            <RingNode
              key={n.id}
              node={n}
              hit={hotNodes.get(n.id)}
              dim={hoveredNeighbours !== null && !hoveredNeighbours.has(n.id)}
              hovered={hovered?.id === n.id}
              bloomDelayMs={settled ? 0 : Math.round(i * bloomStagger)}
              onHover={setHovered}
              onOpen={onOpen}
            />
          ))}
        </g>

        {/* People and agents on the outer ring. */}
        <g>
          {layout.actors.map(a => (
            <ActorMarker key={a.id} actor={a} active={activeActors.has(a.id)} />
          ))}
        </g>

        {/* Centre caption on a soft disc so the chords never cross the text. */}
        <circle
          cx={CENTER}
          cy={CENTER}
          r={70}
          fill="var(--card)"
          opacity={0.92}
          stroke="var(--border)"
        />
        <text
          x={CENTER}
          y={CENTER - 6}
          textAnchor="middle"
          className="fill-foreground text-[44px] font-semibold tabular-nums"
        >
          <AnimatedNumber
            value={layout.nodes.length}
            durationMs={tweenMs}
            disabled={reducedMotion}
          />
        </text>
        <text
          x={CENTER}
          y={CENTER + 22}
          textAnchor="middle"
          className="fill-muted-foreground text-[14px]"
        >
          items ·{' '}
          <AnimatedNumber
            value={shownEdges.length}
            durationMs={tweenMs}
            disabled={reducedMotion}
          />{' '}
          links
        </text>
      </svg>

      {hovered && (
        <NodeTooltip
          node={hovered}
          lastActivity={lastActivity.get(hovered.id)}
        />
      )}
    </div>
  )
}

function NodeTooltip({
  node,
  lastActivity,
}: Readonly<{ node: PlacedNode; lastActivity?: string }>) {
  const left = `${String((node.x / VIEWBOX) * 100)}%`
  const top = `${String((node.y / VIEWBOX) * 100)}%`
  // Flip above the dot in the lower half so the tooltip never leaves the stage.
  const below = node.y < CENTER
  return (
    <div
      role="tooltip"
      className={cn(
        'bg-popover text-popover-foreground animate-in fade-in zoom-in-95 pointer-events-none absolute z-10 w-max max-w-64 -translate-x-1/2 rounded-md border px-3 py-2 text-xs shadow-lg duration-150',
        below ? 'translate-y-4' : '-translate-y-[calc(100%+1rem)]'
      )}
      style={{ left, top }}
    >
      <p className="flex items-center gap-1.5 text-sm font-medium">
        <span
          aria-hidden="true"
          className="inline-block size-2 shrink-0 rounded-full"
          style={{ backgroundColor: RESOURCE_TYPE_COLOR[node.type] }}
        />
        <span className="truncate">{node.title}</span>
      </p>
      <p className="text-muted-foreground mt-0.5">
        {RESOURCE_TYPE_LABEL[node.type]} · {node.degree}{' '}
        {node.degree === 1 ? 'link' : 'links'}
      </p>
      <p className="text-muted-foreground">
        {lastActivity
          ? `Last activity ${new Date(lastActivity).toLocaleTimeString()}`
          : 'No activity this session'}
      </p>
      <p className="text-muted-foreground/80 mt-1 text-[10px]">Click to open</p>
    </div>
  )
}

interface RingNodeProps {
  node: PlacedNode
  hit?: { kind: ActivePulse['kind']; key: string }
  dim: boolean
  hovered: boolean
  bloomDelayMs: number
  onHover: (node: PlacedNode | null) => void
  onOpen: (node: PulseNode) => void
}

/** One resource dot: blooms in, glides on re-layout, haloes on each touch. */
function RingNode({
  node: n,
  hit,
  dim,
  hovered,
  bloomDelayMs,
  onHover,
  onOpen,
}: Readonly<RingNodeProps>) {
  return (
    <g
      className="kp-node cursor-pointer outline-none"
      style={{
        transform: `translate(${n.x.toFixed(1)}px, ${n.y.toFixed(1)}px)`,
      }}
      opacity={dim ? 0.2 : 1}
      onMouseEnter={() => {
        onHover(n)
      }}
      onMouseLeave={() => {
        onHover(null)
      }}
      onFocus={() => {
        onHover(n)
      }}
      onBlur={() => {
        onHover(null)
      }}
      onClick={() => {
        onOpen(n)
      }}
      onKeyDown={ev => {
        if (ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault()
          onOpen(n)
        }
      }}
      tabIndex={0}
      role="button"
      aria-label={`${RESOURCE_TYPE_LABEL[n.type]}: ${n.title}`}
    >
      <g
        className="kp-node-body"
        style={{ '--kp-delay': `${String(bloomDelayMs)}ms` } as CSSProperties}
      >
        {hit && (
          <circle
            key={`halo-${hit.key}`}
            r={n.r}
            fill="none"
            stroke={EVENT_KIND_COLOR[hit.kind]}
            strokeWidth={2.5}
            className="kp-halo"
          />
        )}
        <circle
          r={hit || hovered ? n.r + 2.5 : n.r}
          fill={hit ? EVENT_KIND_COLOR[hit.kind] : RESOURCE_TYPE_COLOR[n.type]}
          stroke="var(--background)"
          strokeWidth={1.5}
          className="kp-node-dot"
        />
        {/* Generous invisible hit target for small dots. */}
        <circle r={Math.max(9, n.r + 3)} fill="transparent" />
      </g>
    </g>
  )
}

/** A person or agent on the outer orbit; lights up while acting. Agents are
 * rounded squares, people circles, so the two read apart at a glance. */
function ActorMarker({
  actor: a,
  active,
}: Readonly<{ actor: PlacedActor; active: boolean }>) {
  const size = active ? 34 : 28
  const fill = active ? 'var(--foreground)' : 'var(--card)'
  const stroke = active ? 'var(--foreground)' : 'var(--border)'
  return (
    <g>
      {a.type === 'agent' ? (
        <rect
          x={a.x - size / 2}
          y={a.y - size / 2}
          width={size}
          height={size}
          rx={8}
          fill={fill}
          stroke={stroke}
          strokeWidth={1.5}
          className="kp-actor-dot"
        />
      ) : (
        <circle
          cx={a.x}
          cy={a.y}
          r={size / 2}
          fill={fill}
          stroke={stroke}
          strokeWidth={1.5}
          className="kp-actor-dot"
        />
      )}
      <text
        x={a.x}
        y={a.y}
        textAnchor="middle"
        dominantBaseline="central"
        className={cn(
          'text-[11px] font-semibold transition-colors',
          active ? 'fill-background' : 'fill-foreground'
        )}
      >
        {initials(a.name)}
      </text>
      <text
        x={a.x}
        y={a.y + 30}
        textAnchor="middle"
        dominantBaseline="middle"
        className={cn(
          'text-[13px] transition-colors',
          active ? 'fill-foreground font-semibold' : 'fill-muted-foreground'
        )}
      >
        {a.name}
      </text>
    </g>
  )
}
