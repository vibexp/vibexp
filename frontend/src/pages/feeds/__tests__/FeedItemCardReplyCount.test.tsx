import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { FeedItem } from '@/services/feedService'

import { FeedItemCard } from '../FeedItemCard'

const mockItem: FeedItem = {
  id: 'item-1',
  team_id: 'team-1',
  feed_id: 'feed-1',
  title: 'Test Title',
  content: 'Test content.',
  excerpt: 'Test excerpt.',
  ai_assistant_name: 'claude-sonnet',
  posted_by_user_id: 'user-1',
  posted_at: new Date(Date.now() - 3600000).toISOString(),
  reply_count: 0,
}

function renderCard(
  props: Partial<React.ComponentProps<typeof FeedItemCard>> = {}
) {
  return render(
    <MemoryRouter>
      <FeedItemCard item={mockItem} {...props} />
    </MemoryRouter>
  )
}

describe('FeedItemCard reply count', () => {
  it('does not render reply count when replyCount is 0', () => {
    renderCard({ replyCount: 0 })
    expect(screen.queryByText(/\d+ repl/i)).not.toBeInTheDocument()
  })

  it('does not render reply count when replyCount is undefined', () => {
    renderCard()
    expect(screen.queryByText(/\d+ repl/i)).not.toBeInTheDocument()
  })

  it('renders count chip on Reply button for replyCount of 1', () => {
    renderCard({ replyCount: 1 })
    const replyBtn = screen.getByRole('button', { name: /reply/i })
    expect(replyBtn).toHaveTextContent(/1/)
  })

  it('renders count chip on Reply button for replyCount of 3', () => {
    renderCard({ replyCount: 3 })
    const replyBtn = screen.getByRole('button', { name: /reply/i })
    expect(replyBtn).toHaveTextContent(/3/)
  })

  it('renders count chip on Reply button for replyCount of 10', () => {
    renderCard({ replyCount: 10 })
    const replyBtn = screen.getByRole('button', { name: /reply/i })
    expect(replyBtn).toHaveTextContent(/10/)
  })

  it('shows "View full post" link when replyCount > 0', () => {
    renderCard({ replyCount: 2 })
    expect(
      screen.getByRole('link', { name: /view full post/i })
    ).toBeInTheDocument()
  })
})
