import { describe, expect, it } from 'vitest'

import { capNodes, CENTER, layoutRing, NODE_RING_RADIUS } from '../layout'
import {
  initialPulseState,
  PULSE_DURATION_MS,
  pulseReducer,
  sortEventsByTime,
  spreadDelays,
} from '../pulseState'
import type { PulseEvent, PulseNode } from '../types'

const node = (id: string, type: PulseNode['type'], degree = 0): PulseNode => ({
  id,
  type,
  title: id,
  degree,
})

const event = (
  over: Partial<PulseEvent> & Pick<PulseEvent, 'id'>
): PulseEvent => ({
  kind: 'read',
  at: '2026-10-19T10:00:00Z',
  actor: { type: 'agent', id: 'cc', name: 'Claude Code', source: 'mcp' },
  resource: { type: 'memory', id: 'm1', title: 'Deploy runbook' },
  ...over,
})

describe('spreadDelays', () => {
  it('spreads events evenly across the interval', () => {
    expect(spreadDelays(4, 10000)).toEqual([0, 2500, 5000, 7500])
    expect(spreadDelays(0, 10000)).toEqual([])
  })
})

describe('sortEventsByTime', () => {
  it('orders oldest first', () => {
    const sorted = sortEventsByTime([
      event({ id: 'b', at: '2026-10-19T10:00:02Z' }),
      event({ id: 'a', at: '2026-10-19T10:00:01Z' }),
    ])
    expect(sorted.map(e => e.id)).toEqual(['a', 'b'])
  })
})

describe('pulseReducer', () => {
  it('adds unknown resources and actors from live events and animates them', () => {
    const s = pulseReducer(initialPulseState(), {
      type: 'event',
      event: event({ id: 'e1' }),
      now: 1000,
      animate: true,
    })
    expect(s.nodes.get('m1')?.title).toBe('Deploy runbook')
    expect(s.actors.get('cc')?.name).toBe('Claude Code')
    expect(s.pulses).toHaveLength(1)
    expect(s.touches.get('m1')).toBe(1)
    expect(s.feed[0].id).toBe('e1')
  })

  it('ignores an event it has already applied', () => {
    let s = initialPulseState()
    for (let i = 0; i < 2; i++) {
      s = pulseReducer(s, {
        type: 'event',
        event: event({ id: 'e1' }),
        now: 0,
        animate: false,
      })
    }
    expect(s.feed).toHaveLength(1)
    expect(s.touches.get('m1')).toBe(1)
  })

  it('records a link as an edge and bumps both degrees once', () => {
    const link = event({
      id: 'l1',
      kind: 'link',
      detail: {
        relation_type: 'built-from',
        to: { type: 'artifact', id: 'a1', title: 'Spec' },
      },
    })
    let s = pulseReducer(initialPulseState(), {
      type: 'event',
      event: link,
      now: 0,
      animate: true,
    })
    s = pulseReducer(s, {
      type: 'event',
      event: { ...link, id: 'l2' },
      now: 0,
      animate: true,
    })
    expect(s.edges).toEqual([
      { from: 'm1', to: 'a1', relation_type: 'built-from' },
    ])
    expect(s.nodes.get('m1')?.degree).toBe(1)
    expect(s.nodes.get('a1')?.degree).toBe(1)
    expect(s.pulses[0].toId).toBe('a1')
  })

  it('keeps event-discovered nodes when a snapshot arrives', () => {
    let s = pulseReducer(initialPulseState(), {
      type: 'event',
      event: event({ id: 'e1' }),
      now: 0,
      animate: false,
    })
    s = pulseReducer(s, {
      type: 'snapshot',
      snapshot: { nodes: [node('p1', 'prompt')], edges: [], actors: [] },
    })
    expect([...s.nodes.keys()].sort()).toEqual(['m1', 'p1'])
  })

  it('expires pulses after their duration', () => {
    let s = pulseReducer(initialPulseState(), {
      type: 'event',
      event: event({ id: 'e1' }),
      now: 0,
      animate: true,
    })
    s = pulseReducer(s, { type: 'prune', now: PULSE_DURATION_MS - 1 })
    expect(s.pulses).toHaveLength(1)
    s = pulseReducer(s, { type: 'prune', now: PULSE_DURATION_MS })
    expect(s.pulses).toHaveLength(0)
  })
})

describe('layoutRing', () => {
  it('groups nodes into contiguous arcs per type on the ring', () => {
    const layout = layoutRing(
      [
        node('a', 'artifact'),
        node('m', 'memory'),
        node('m2', 'memory'),
        node('p', 'prompt'),
      ],
      []
    )
    expect(layout.arcs.map(a => [a.type, a.count])).toEqual([
      ['memory', 2],
      ['artifact', 1],
      ['prompt', 1],
    ])
    for (const n of layout.nodes) {
      expect(Math.hypot(n.x - CENTER, n.y - CENTER)).toBeCloseTo(
        NODE_RING_RADIUS
      )
    }
    const angles = layout.nodes.map(n => n.angle)
    expect([...angles].sort((x, y) => x - y)).toEqual(angles)
  })

  it('is deterministic regardless of input order', () => {
    const nodes = [
      node('b', 'memory'),
      node('a', 'memory'),
      node('c', 'prompt'),
    ]
    const one = layoutRing(nodes, []).nodes.map(n => [n.id, n.angle])
    const two = layoutRing([...nodes].reverse(), []).nodes.map(n => [
      n.id,
      n.angle,
    ])
    expect(two).toEqual(one)
  })
})

describe('capNodes', () => {
  it('keeps recent nodes first, then the best connected', () => {
    const nodes = [
      node('a', 'memory', 9),
      node('b', 'memory', 1),
      node('c', 'memory', 5),
    ]
    expect(capNodes(nodes, new Set(['b']), 2).map(n => n.id)).toEqual([
      'b',
      'a',
    ])
  })
})
