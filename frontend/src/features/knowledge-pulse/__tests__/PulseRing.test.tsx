import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { PulseRing } from '../PulseRing'
import type { ActivePulse } from '../pulseState'
import { snapshot } from './fixtures'

function renderRing(over: Partial<Parameters<typeof PulseRing>[0]> = {}) {
  const snap = snapshot()
  const onOpen = vi.fn()
  const utils = render(
    <PulseRing
      nodes={snap.nodes}
      edges={snap.edges}
      actors={snap.actors}
      pulses={[]}
      lastActivity={new Map()}
      reducedMotion={false}
      live
      tweenMs={1000}
      onOpen={onOpen}
      {...over}
    />
  )
  return { ...utils, onOpen }
}

describe('PulseRing', () => {
  it('summarises the graph for assistive tech and labels every node', () => {
    renderRing()
    expect(
      screen.getByRole('img', {
        name: 'Knowledge ring: 4 items, 2 links, 2 people and agents',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Memories: Deploy runbook' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Artifacts: Incident report' })
    ).toBeInTheDocument()
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
    // Agents show initials, people too.
    expect(screen.getByText('CC')).toBeInTheDocument()
    expect(screen.getByText('AL')).toBeInTheDocument()
  })

  it('shows an empty state with no nodes', () => {
    renderRing({ nodes: [], edges: [], actors: [] })
    expect(screen.getByText(/No knowledge yet/)).toBeInTheDocument()
  })

  it('hovering a node shows its tooltip; leaving hides it', async () => {
    renderRing({ lastActivity: new Map([['m1', '2026-10-19T10:00:00Z']]) })
    const dot = screen.getByRole('button', { name: 'Memories: Deploy runbook' })
    await userEvent.hover(dot)
    const tip = screen.getByRole('tooltip')
    expect(tip).toHaveTextContent('Deploy runbook')
    expect(tip).toHaveTextContent('2 links')
    expect(tip).toHaveTextContent('Last activity')
    await userEvent.unhover(dot)
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('a node with one link and no activity says so', () => {
    renderRing()
    fireEvent.focus(
      screen.getByRole('button', { name: 'Artifacts: Incident report' })
    )
    const tip = screen.getByRole('tooltip')
    expect(tip).toHaveTextContent('1 link')
    expect(tip).toHaveTextContent('No activity this session')
    fireEvent.blur(
      screen.getByRole('button', { name: 'Artifacts: Incident report' })
    )
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('click, Enter and Space open the resource', async () => {
    const { onOpen } = renderRing()
    const dot = screen.getByRole('button', { name: 'Prompts: Code review' })
    await userEvent.click(dot)
    fireEvent.keyDown(dot, { key: 'Enter' })
    fireEvent.keyDown(dot, { key: ' ' })
    fireEvent.keyDown(dot, { key: 'a' })
    expect(onOpen).toHaveBeenCalledTimes(3)
    expect(onOpen.mock.calls[0][0]).toMatchObject({
      id: 'p1',
      slug: 'code-review',
    })
  })

  it('a pulse draws a halo on the node, both ends of a link, and a beam from the actor', () => {
    const pulses: ActivePulse[] = [
      {
        key: 'e1',
        nodeId: 'm1',
        toId: 'a1',
        actorId: 'agent:claude-code',
        kind: 'link',
        startedAt: 0,
      },
      {
        key: 'e2',
        nodeId: 'p1',
        actorId: 'nobody',
        kind: 'read',
        startedAt: 0,
      },
    ]
    const { container } = renderRing({ pulses })
    expect(container.querySelectorAll('.kp-halo')).toHaveLength(3)
    // Only the pulse whose actor is on the ring gets a beam.
    expect(container.querySelectorAll('.kp-beam')).toHaveLength(1)
  })

  it('stops staggering the bloom once the first paint has settled', () => {
    vi.useFakeTimers()
    try {
      const { container } = renderRing({ reducedMotion: true, live: false })
      const bodies = () => [
        ...container.querySelectorAll<SVGGElement>('.kp-node-body'),
      ]
      expect(
        bodies().some(b => b.style.getPropertyValue('--kp-delay') !== '0ms')
      ).toBe(true)
      act(() => {
        vi.advanceTimersByTime(2000)
      })
      expect(
        bodies().every(b => b.style.getPropertyValue('--kp-delay') === '0ms')
      ).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})
