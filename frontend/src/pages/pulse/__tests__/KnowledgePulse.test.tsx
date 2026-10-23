/**
 * The Knowledge Pulse page end to end against a mocked fetch: the first load
 * (snapshot + silent backfill), play/pause polling with the cursor, replay of
 * a poll's events across the interval, the hidden-tab and error states, and
 * opening a resource. The real hook, reducer and ring run; only the network
 * is faked.
 */
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  pulseEvent,
  response,
} from '@/features/knowledge-pulse/__tests__/fixtures'

const mockFetch = vi.hoisted(() => vi.fn())
vi.mock('@/features/knowledge-pulse/api', () => ({
  fetchKnowledgePulse: mockFetch,
}))
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1', name: 'Team' } }),
}))

import { KnowledgePulse } from '../KnowledgePulse'

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    get: () => hidden,
  })
  act(() => {
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

function renderPage(path = '/pulse') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/pulse" element={<KnowledgePulse />} />
        <Route path="/prompts/:slug" element={<p>prompt page</p>} />
        <Route path="/memories/:id" element={<p>memory page</p>} />
        <Route path="/feeds/items/:id" element={<p>feed item page</p>} />
      </Routes>
    </MemoryRouter>
  )
}

/** The live activity panel (its rows, not the most-touched list's). */
const feedPanel = () =>
  screen.getByRole('heading', { name: 'Live activity' }).parentElement!
    .parentElement!

const playButton = () => screen.getByRole('button', { name: /^(Play|Pause)$/ })

describe('KnowledgePulse page', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: false })
    mockFetch.mockReset()
    // Reduced motion: counters render their value at once, so assertions
    // need not wait out a tween.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
    setHidden(false)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads the ring, backfills the feed silently and shows today', async () => {
    mockFetch.mockResolvedValueOnce(
      response({
        events: [pulseEvent({ id: 'old', at: '2026-10-19T09:59:00Z' })],
      })
    )
    renderPage()
    await flush()

    expect(mockFetch).toHaveBeenCalledWith(
      'team-1',
      {},
      expect.any(AbortSignal)
    )
    expect(
      screen.getByRole('img', { name: /4 items, 2 links/ })
    ).toBeInTheDocument()
    expect(screen.getByText('500')).toBeInTheDocument()
    expect(within(feedPanel()).getByText('Claude Code')).toBeInTheDocument()
    // Backfilled events do not animate.
    expect(document.querySelectorAll('.kp-halo')).toHaveLength(0)
    expect(screen.getByText('Paused')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Press play/ })
    ).toBeInTheDocument()
  })

  it('plays: polls with the cursor, replays events across the interval, pauses', async () => {
    mockFetch.mockResolvedValueOnce(response())
    renderPage()
    await flush()

    mockFetch.mockResolvedValue(
      response({
        cursor: 'c2',
        snapshot: undefined,
        events: [
          pulseEvent({ id: 'n1', at: '2026-10-19T10:00:01Z' }),
          pulseEvent({
            id: 'n2',
            at: '2026-10-19T10:00:02Z',
            kind: 'write',
            detail: { action: 'update' },
          }),
        ],
        stats_today: {
          reads: 501,
          writes: 41,
          links: 9,
          compounding_score: 15,
        },
      })
    )
    fireEvent.click(playButton())
    await flush()

    expect(mockFetch).toHaveBeenLastCalledWith(
      'team-1',
      { since: '2026-10-19T10:00:00Z' },
      expect.any(AbortSignal)
    )
    expect(screen.getByText('Live · every 10s')).toBeInTheDocument()
    expect(screen.getByText('501')).toBeInTheDocument()
    // First event plays at once, the second half an interval later.
    expect(within(feedPanel()).getAllByRole('listitem')).toHaveLength(1)
    expect(document.querySelectorAll('.kp-halo').length).toBeGreaterThan(0)
    await flush(5000)
    expect(within(feedPanel()).getByText('updated')).toBeInTheDocument()

    // The next poll uses the new cursor.
    await flush(5000)
    expect(mockFetch).toHaveBeenLastCalledWith(
      'team-1',
      { since: 'c2' },
      expect.any(AbortSignal)
    )

    fireEvent.click(playButton())
    expect(screen.getByText('Paused')).toBeInTheDocument()
  })

  it('animates only the newest 30 events of a big batch', async () => {
    mockFetch.mockResolvedValueOnce(response())
    renderPage()
    await flush()
    const many = Array.from({ length: 35 }, (_, i) =>
      pulseEvent({
        id: `b${String(i)}`,
        at: new Date(
          Date.parse('2026-10-19T10:00:00Z') + i * 1000
        ).toISOString(),
      })
    )
    mockFetch.mockResolvedValueOnce(
      response({ cursor: 'c2', snapshot: undefined, events: many })
    )
    mockFetch.mockResolvedValue(response({ cursor: 'c3', snapshot: undefined }))
    fireEvent.click(playButton())
    await flush()
    // 5 silent + the first animated one already in the feed.
    expect(within(feedPanel()).getAllByRole('listitem')).toHaveLength(6)
  })

  it('changing the interval polls on the new cadence', async () => {
    mockFetch.mockResolvedValue(response({ snapshot: undefined }))
    renderPage('/pulse?autoplay=1')
    await flush()
    expect(screen.getByText('Live · every 10s')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: '30s' }))
    await flush()
    expect(screen.getByText('Live · every 30s')).toBeInTheDocument()
    const calls = mockFetch.mock.calls.length
    await flush(10000)
    expect(mockFetch.mock.calls).toHaveLength(calls)
    await flush(20000)
    expect(mockFetch.mock.calls).toHaveLength(calls + 1)
  })

  it('Space toggles play, but not while a control has focus', async () => {
    mockFetch.mockResolvedValue(response({ snapshot: undefined }))
    renderPage()
    await flush()
    fireEvent.keyDown(window, { code: 'Space' })
    expect(screen.getByText('Live · every 10s')).toBeInTheDocument()
    fireEvent.keyDown(playButton(), { code: 'Space' })
    expect(screen.getByText('Live · every 10s')).toBeInTheDocument()
    fireEvent.keyDown(window, { code: 'KeyA' })
    fireEvent.keyDown(window, { code: 'Space' })
    expect(screen.getByText('Paused')).toBeInTheDocument()
  })

  it('suspends while the tab is hidden', async () => {
    mockFetch.mockResolvedValue(response({ snapshot: undefined }))
    renderPage('/pulse?autoplay=1')
    await flush()
    setHidden(true)
    expect(
      screen.getByText('Paused while the tab is hidden')
    ).toBeInTheDocument()
    const calls = mockFetch.mock.calls.length
    await flush(30000)
    expect(mockFetch.mock.calls).toHaveLength(calls)
  })

  it('reports a failed first load and a failing poll', async () => {
    mockFetch.mockRejectedValueOnce(new Error('boom'))
    const { unmount } = renderPage()
    await flush()
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't load the knowledge pulse: boom"
    )
    unmount()

    mockFetch.mockResolvedValueOnce(response())
    mockFetch.mockRejectedValue(new Error('down'))
    renderPage('/pulse?autoplay=1')
    await flush()
    expect(screen.getByText('Connection problem, retrying')).toBeInTheDocument()
  })

  it('opens resources from the ring and the feed', async () => {
    mockFetch.mockResolvedValueOnce(
      response({
        events: [
          pulseEvent({
            id: 'f',
            kind: 'feed_post',
            resource: { type: 'feed_item', id: 'f1', title: 'Shipped it' },
          }),
        ],
      })
    )
    const { unmount } = renderPage()
    await flush()
    fireEvent.click(
      screen.getByRole('button', { name: 'Prompts: Code review' })
    )
    expect(screen.getByText('prompt page')).toBeInTheDocument()
    unmount()

    mockFetch.mockResolvedValueOnce(
      response({
        events: [
          pulseEvent({
            id: 'f',
            kind: 'feed_post',
            resource: { type: 'feed_item', id: 'f1', title: 'Shipped it' },
          }),
        ],
      })
    )
    renderPage()
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Shipped it' }))
    expect(screen.getByText('feed item page')).toBeInTheDocument()
  })

  it('goes full screen and back', async () => {
    mockFetch.mockResolvedValue(response())
    renderPage()
    await flush()
    const request = vi.fn(function (this: HTMLElement) {
      Object.defineProperty(document, 'fullscreenElement', {
        configurable: true,
        value: this,
      })
      document.dispatchEvent(new Event('fullscreenchange'))
      return Promise.resolve()
    })
    const exit = vi.fn(() => {
      Object.defineProperty(document, 'fullscreenElement', {
        configurable: true,
        value: null,
      })
      document.dispatchEvent(new Event('fullscreenchange'))
      return Promise.resolve()
    })
    HTMLElement.prototype.requestFullscreen = request
    document.exitFullscreen = exit

    fireEvent.click(screen.getByRole('button', { name: 'Full screen' }))
    expect(request).toHaveBeenCalled()
    // In TV mode the controls move into the stage and the panels hide.
    expect(
      screen.getAllByRole('button', { name: 'Exit full screen' }).length
    ).toBeGreaterThan(0)
    expect(screen.queryByText('Most touched')).not.toBeInTheDocument()

    fireEvent.click(
      screen.getAllByRole('button', { name: 'Exit full screen' })[0]
    )
    expect(exit).toHaveBeenCalled()
    expect(screen.getByText('Most touched')).toBeInTheDocument()
  })
})
