import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { EventFeed, StatTiles, TopItems } from '../PulsePanels'
import { node, pulseEvent } from './fixtures'

describe('EventFeed', () => {
  it('shows a quiet state with no events', () => {
    render(<EventFeed events={[]} onOpen={vi.fn()} />)
    expect(screen.getByText(/Quiet for now/)).toBeInTheDocument()
  })

  it('writes each event as a sentence and opens its resource', async () => {
    const onOpen = vi.fn()
    const events = [
      pulseEvent({
        id: 'l1',
        kind: 'link',
        actor: { type: 'user', id: 'u1', name: 'Ada', source: 'web' },
        detail: {
          relation_type: 'supersedes',
          to: { type: 'artifact', id: 'a1', title: 'Spec' },
        },
      }),
      pulseEvent({ id: 'r1' }),
    ]
    render(<EventFeed events={events} onOpen={onOpen} />)
    expect(screen.getByText('2 recent')).toBeInTheDocument()
    const [link, read] = screen.getAllByRole('listitem')
    expect(link).toHaveTextContent('Ada linked Deploy runbook to Spec')
    expect(link).toHaveTextContent('supersedes')
    expect(read).toHaveTextContent('Claude Code read Deploy runbook')
    expect(read).toHaveTextContent('via MCP')
    await userEvent.click(
      within(read).getByRole('button', { name: 'Deploy runbook' })
    )
    expect(onOpen).toHaveBeenCalledWith(events[1].resource)
  })
})

describe('StatTiles', () => {
  const sample = (t: number, reads: number) => ({
    t,
    reads,
    writes: 1,
    links: 1,
    compounding: 1,
  })

  it('shows dashes before the first load', () => {
    render(<StatTiles stats={null} history={[]} tweenMs={0} reducedMotion />)
    expect(screen.getAllByText('—')).toHaveLength(4)
  })

  it("shows today's totals and what grew this session", () => {
    render(
      <StatTiles
        stats={{ reads: 512, writes: 1, links: 1, compounding_score: 1 }}
        history={[sample(0, 500), sample(1, 512)]}
        tweenMs={0}
        reducedMotion
      />
    )
    expect(screen.getByText('Reads today')).toBeInTheDocument()
    expect(screen.getByText('512')).toBeInTheDocument()
    expect(screen.getByText(/this session/)).toHaveTextContent(
      '+12 this session'
    )
  })
})

describe('TopItems', () => {
  it('shows a placeholder until something is touched', () => {
    render(<TopItems nodes={new Map()} touches={new Map()} onOpen={vi.fn()} />)
    expect(screen.getByText('Nothing yet.')).toBeInTheDocument()
  })

  it('ranks the most-touched items and skips ones no longer on the ring', async () => {
    const onOpen = vi.fn()
    const a = node('a', 'memory', 0, { title: 'Alpha' })
    const b = node('b', 'prompt', 0, { title: 'Beta' })
    render(
      <TopItems
        nodes={
          new Map([
            ['a', a],
            ['b', b],
          ])
        }
        touches={
          new Map([
            ['a', 2],
            ['b', 5],
            ['gone', 9],
          ])
        }
        onOpen={onOpen}
      />
    )
    const rows = screen.getAllByRole('button')
    expect(rows.map(r => r.textContent)).toEqual([
      '1BetaPrompts5',
      '2AlphaMemories2',
    ])
    await userEvent.click(rows[1])
    expect(onOpen).toHaveBeenCalledWith(a)
  })
})
