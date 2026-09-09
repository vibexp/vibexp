/**
 * Behavioural cover for the redirects #920 left behind.
 *
 * `routeCutover.test.ts` proves what `routes.tsx` DECLARES; this proves what
 * the redirect elements DO — that they land on the normalised path, carry the
 * param across, and use `replace` so Back leaves the page instead of bouncing
 * through the redirect.
 *
 * It mounts the exported redirect components rather than `AppRoutes` (which
 * would need stubs for ~40 pages). Only the `path=` strings are restated here,
 * and those are pinned against the real source by `routeCutover.test.ts` — so
 * the pair is not circular.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router'

import { AgentsAddRedirect, FeedItemRedirect } from '../routes'

function LocationProbe() {
  const location = useLocation()
  const navigate = useNavigate()
  return (
    <>
      <div data-testid="location">{location.pathname}</div>
      <button
        type="button"
        onClick={() => {
          void navigate(-1)
        }}
      >
        history back
      </button>
    </>
  )
}

function renderAt(entries: string[]) {
  return render(
    <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
      <LocationProbe />
      <Routes>
        <Route path="/agents" element={<div>Agents list</div>} />
        <Route path="/agents/add" element={<AgentsAddRedirect />} />
        <Route
          path="/agents/new"
          element={<div data-testid="agent-editor">Agent editor</div>}
        />
        <Route path="/feeds" element={<div>Feeds</div>} />
        <Route path="/feed-items/:itemId" element={<FeedItemRedirect />} />
        <Route
          path="/feeds/items/:itemId"
          element={<div data-testid="feed-item">Feed item</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

const currentPath = () => screen.getByTestId('location').textContent

describe('retired resource routes redirect (#920)', () => {
  it('/agents/add lands on /agents/new', async () => {
    renderAt(['/agents/add'])

    await waitFor(() => {
      expect(currentPath()).toBe('/agents/new')
    })
    expect(screen.getByTestId('agent-editor')).toBeInTheDocument()
  })

  it('/feed-items/:itemId carries the id onto /feeds/items/:itemId', async () => {
    renderAt(['/feed-items/item-1'])

    await waitFor(() => {
      expect(currentPath()).toBe('/feeds/items/item-1')
    })
    expect(screen.getByTestId('feed-item')).toBeInTheDocument()
  })

  it('re-encodes a feed item id that needs it', async () => {
    // `useParams` hands back a decoded segment, so a redirect that does not
    // re-encode would emit a raw `/` or space into the new URL.
    renderAt(['/feed-items/item%2Fone'])

    await waitFor(() => {
      expect(currentPath()).toBe('/feeds/items/item%2Fone')
    })
  })

  it.each([
    ['/agents', '/agents/add'],
    ['/feeds', '/feed-items/item-1'],
  ])(
    'replaces the retired entry, so Back from it returns to %s',
    async (previous, retired) => {
      renderAt([previous, retired])

      await waitFor(() => {
        expect(currentPath()).not.toBe(retired)
      })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'history back' }))

      await waitFor(() => {
        expect(currentPath()).toBe(previous)
      })
    }
  )
})
