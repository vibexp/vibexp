import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import type { AdminUserTimelineEvent } from '@/services/adminService'

const mockGetUserTimeline = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getUserTimeline: (...args: unknown[]) => mockGetUserTimeline(...args),
  },
}))

import { UserActivityTab } from '../detail/UserActivityTab'

function event(
  overrides: Partial<AdminUserTimelineEvent> = {}
): AdminUserTimelineEvent {
  return {
    resource_type: 'prompt',
    action: 'created',
    team_id: 't1',
    team_name: 'Engineering',
    project_id: 'p1',
    project_name: 'Core',
    resource_short_id: '3f2a9c1e',
    occurred_at: '2026-09-20T10:00:00Z',
    ...overrides,
  }
}

function renderTab() {
  return render(
    <MemoryRouter>
      <UserActivityTab userId="u1" />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('requests the first page without a cursor and renders opaque rows', async () => {
  mockGetUserTimeline.mockResolvedValue({
    items: [
      event(),
      event({
        resource_type: 'feed_item',
        action: 'updated',
        project_id: null,
        project_name: null,
        resource_short_id: 'aa11bb22',
      }),
    ],
    next_cursor: null,
  })
  renderTab()

  expect(await screen.findByText('3f2a9c1e')).toBeInTheDocument()
  expect(mockGetUserTimeline).toHaveBeenCalledTimes(1)
  expect(mockGetUserTimeline).toHaveBeenCalledWith('u1', { limit: 50 })
  expect(screen.getByText('Feed item')).toBeInTheDocument()
  expect(screen.getByText('updated')).toBeInTheDocument()
  expect(screen.getByText('—')).toBeInTheDocument()
  expect(
    screen.getAllByRole('link', { name: 'Engineering' })[0]
  ).toHaveAttribute('href', '/admin/teams/t1')
  expect(
    screen.queryByRole('button', { name: 'Load more' })
  ).not.toBeInTheDocument()
})

it('never renders a title, even when the payload carries one', async () => {
  const leaky = {
    ...event(),
    title: 'Secret roadmap prompt',
  } as AdminUserTimelineEvent
  mockGetUserTimeline.mockResolvedValue({ items: [leaky], next_cursor: null })
  renderTab()

  await screen.findByText('3f2a9c1e')
  expect(screen.queryByText(/Secret roadmap prompt/)).not.toBeInTheDocument()
})

it('appends the next page on Load more, following the cursor', async () => {
  mockGetUserTimeline
    .mockResolvedValueOnce({ items: [event()], next_cursor: 'c2' })
    .mockResolvedValueOnce({
      items: [event({ resource_short_id: 'bbbbbbbb' })],
      next_cursor: null,
    })
  renderTab()

  await userEvent.click(
    await screen.findByRole('button', { name: 'Load more' })
  )

  expect(await screen.findByText('bbbbbbbb')).toBeInTheDocument()
  expect(screen.getByText('3f2a9c1e')).toBeInTheDocument()
  expect(mockGetUserTimeline).toHaveBeenLastCalledWith('u1', {
    cursor: 'c2',
    limit: 50,
  })
  expect(
    screen.queryByRole('button', { name: 'Load more' })
  ).not.toBeInTheDocument()
})

it('keeps the loaded rows when Load more fails', async () => {
  mockGetUserTimeline
    .mockResolvedValueOnce({ items: [event()], next_cursor: 'c2' })
    .mockRejectedValueOnce(new Error('boom'))
  renderTab()

  await userEvent.click(
    await screen.findByRole('button', { name: 'Load more' })
  )

  expect(await screen.findByRole('alert')).toHaveTextContent('boom')
  expect(screen.getByText('3f2a9c1e')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Load more' })).toBeEnabled()
})

it('shows the empty state', async () => {
  mockGetUserTimeline.mockResolvedValue({ items: [], next_cursor: null })
  renderTab()

  expect(
    await screen.findByText(
      'This user has not created or updated any resources.'
    )
  ).toBeInTheDocument()
})

it('shows the loading then error state', async () => {
  mockGetUserTimeline.mockRejectedValue(new Error('timeline down'))
  renderTab()

  expect(screen.getByTestId('activity-loading')).toBeInTheDocument()
  await waitFor(() => {
    expect(screen.queryByTestId('activity-loading')).not.toBeInTheDocument()
  })
  expect(screen.getByText('Failed to load activity')).toBeInTheDocument()
  expect(screen.getByText('timeline down')).toBeInTheDocument()
})
