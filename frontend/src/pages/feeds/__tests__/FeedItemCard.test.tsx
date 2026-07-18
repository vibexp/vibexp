import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

import { FeedItemCard } from '../FeedItemCard'

const mockItem: FeedItem = {
  id: 'item-1',
  team_id: 'team-1',
  feed_id: 'feed-1',
  title: 'Sprint Retrospective',
  content: '## Summary\nThe sprint went well.',
  excerpt: 'The sprint went well and all tasks were completed.',
  ai_assistant_name: 'claude-sonnet-4-5',
  posted_by_user_id: 'user-1',
  posted_at: new Date(Date.now() - 3600000).toISOString(), // 1 hour ago
  reply_count: 0,
}

const archivedItem: FeedItem = {
  ...mockItem,
  id: 'item-2',
  archived_at: new Date().toISOString(),
}

const userPostItem: FeedItem = {
  ...mockItem,
  id: 'item-user',
  ai_assistant_name: 'User Post',
  posted_by_user_id: 'user-1',
}

const aliceMember: TeamMember = {
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice Smith',
  role: 'member',
  joined_at: new Date().toISOString(),
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

describe('FeedItemCard', () => {
  it.each([
    ['item title', 'Sprint Retrospective'],
    ['AI assistant name in the header for AI items', 'claude-sonnet-4-5'],
    ['the AI badge next to the assistant name for AI items', 'AI'],
  ])('renders %s', (_what, text) => {
    renderCard()
    expect(screen.getByText(text)).toBeInTheDocument()
  })

  it('renders excerpt', () => {
    renderCard()
    expect(
      screen.getByText(/The sprint went well and all tasks were completed/)
    ).toBeInTheDocument()
  })

  it('renders the matched provider icon for AI items', () => {
    renderCard()
    expect(screen.getByTestId('ai-assistant-icon-claude')).toBeInTheDocument()
    expect(screen.getByAltText(/claude logo/i)).toBeInTheDocument()
  })

  it('renders relative time in the header', () => {
    renderCard()
    expect(screen.getByText(/ago/)).toBeInTheDocument()
  })

  it('renders feed name in header when feedName is provided', () => {
    renderCard({ feedName: 'Product Updates' })
    expect(screen.getByText('Product Updates')).toBeInTheDocument()
  })

  it('does not render feed name section when feedName is omitted', () => {
    renderCard()
    expect(screen.queryByText('Product Updates')).not.toBeInTheDocument()
  })

  it('renders project name when provided with a project_id on the item', () => {
    const itemWithProject: FeedItem = { ...mockItem, project_id: 'proj-1' }
    render(
      <MemoryRouter>
        <FeedItemCard item={itemWithProject} projectName="My Project" />
      </MemoryRouter>
    )
    expect(screen.getByText('My Project')).toBeInTheDocument()
  })

  it('does not render project name when project_id is absent', () => {
    renderCard({ projectName: 'My Project' })
    expect(screen.queryByText('My Project')).not.toBeInTheDocument()
  })

  it('title is a link pointing to the correct feed-item URL', () => {
    renderCard()
    const link = screen.getByRole('link', { name: 'Sprint Retrospective' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/feed-items/item-1')
  })

  it('does not show archive button when no onArchive callback', () => {
    renderCard()
    expect(screen.queryByLabelText('Archive')).not.toBeInTheDocument()
  })

  it('shows archive button when onArchive is provided', () => {
    renderCard({ onArchive: jest.fn() })
    expect(screen.getByLabelText('Archive')).toBeInTheDocument()
  })

  it('calls onArchive when archive button clicked', () => {
    const onArchive = jest.fn().mockResolvedValue(undefined)
    renderCard({ onArchive })
    fireEvent.click(screen.getByLabelText('Archive'))
    expect(onArchive).toHaveBeenCalledWith(mockItem)
  })

  it('shows delete button when onDelete is provided', () => {
    renderCard({ onDelete: jest.fn() })
    expect(screen.getByLabelText('Delete')).toBeInTheDocument()
  })

  it('calls onDelete when delete button clicked', () => {
    const onDelete = jest.fn()
    renderCard({ onDelete })
    fireEvent.click(screen.getByLabelText('Delete'))
    expect(onDelete).toHaveBeenCalledWith(mockItem)
  })

  it('shows archived badge for archived items', () => {
    render(
      <MemoryRouter>
        <FeedItemCard item={archivedItem} />
      </MemoryRouter>
    )
    expect(screen.getByText('Archived')).toBeInTheDocument()
  })

  it('shows unarchive button for archived items', () => {
    render(
      <MemoryRouter>
        <FeedItemCard item={archivedItem} onUnarchive={jest.fn()} />
      </MemoryRouter>
    )
    expect(screen.getByLabelText('Unarchive')).toBeInTheDocument()
  })

  it('action buttons are inside a group that applies opacity transition', () => {
    const { container } = renderCard({ onDelete: jest.fn() })
    // The outer post div carries the "group" class
    const groupEl = container.querySelector('.group')
    expect(groupEl).toBeInTheDocument()
    // The actions wrapper has opacity-0 / group-hover:opacity-100
    const actionsWrapper = container.querySelector(
      '.opacity-0.group-hover\\:opacity-100'
    )
    expect(actionsWrapper).toBeInTheDocument()
  })

  it('clicking the delete button stops propagation so the row is not also activated', () => {
    const onDelete = jest.fn()
    render(
      <MemoryRouter>
        <FeedItemCard item={mockItem} onDelete={onDelete} />
      </MemoryRouter>
    )

    const deleteButton = screen.getByLabelText('Delete')
    let propagationStopped = false
    const originalStopPropagation = Event.prototype.stopPropagation
    Event.prototype.stopPropagation = function () {
      propagationStopped = true
      originalStopPropagation.call(this)
    }

    try {
      fireEvent.click(deleteButton)
    } finally {
      Event.prototype.stopPropagation = originalStopPropagation
    }

    expect(onDelete).toHaveBeenCalledWith(mockItem)
    expect(propagationStopped).toBe(true)
  })

  describe('user-posted items', () => {
    it('renders the team member name (not "User Post") when member resolves', () => {
      render(
        <MemoryRouter>
          <FeedItemCard item={userPostItem} member={aliceMember} />
        </MemoryRouter>
      )
      expect(screen.getByText('Alice Smith')).toBeInTheDocument()
      expect(screen.queryByText('User Post')).not.toBeInTheDocument()
    })

    it('renders an initial avatar (not an AI icon) for user posts', () => {
      render(
        <MemoryRouter>
          <FeedItemCard item={userPostItem} member={aliceMember} />
        </MemoryRouter>
      )
      // Alice → A
      expect(screen.getByText('A')).toBeInTheDocument()
      expect(
        screen.queryByTestId(/^ai-assistant-icon-/)
      ).not.toBeInTheDocument()
      expect(screen.queryByText('AI')).not.toBeInTheDocument()
    })

    it('falls back to "Unknown user" when no member is resolved', () => {
      render(
        <MemoryRouter>
          <FeedItemCard item={userPostItem} />
        </MemoryRouter>
      )
      expect(screen.getByText('Unknown user')).toBeInTheDocument()
      expect(screen.getByText('?')).toBeInTheDocument()
    })
  })

  describe('AI provider icons', () => {
    it('renders openai icon for OpenAI/Codex assistants', () => {
      const codexItem: FeedItem = { ...mockItem, ai_assistant_name: 'Codex' }
      render(
        <MemoryRouter>
          <FeedItemCard item={codexItem} />
        </MemoryRouter>
      )
      expect(screen.getByTestId('ai-assistant-icon-openai')).toBeInTheDocument()
    })

    it('renders gemini icon for google assistants', () => {
      const gItem: FeedItem = { ...mockItem, ai_assistant_name: 'Google Bard' }
      render(
        <MemoryRouter>
          <FeedItemCard item={gItem} />
        </MemoryRouter>
      )
      expect(screen.getByTestId('ai-assistant-icon-gemini')).toBeInTheDocument()
    })

    it('falls back to generic icon for unknown providers', () => {
      const mItem: FeedItem = {
        ...mockItem,
        ai_assistant_name: 'mistral-large',
      }
      render(
        <MemoryRouter>
          <FeedItemCard item={mItem} />
        </MemoryRouter>
      )
      expect(
        screen.getByTestId('ai-assistant-icon-generic')
      ).toBeInTheDocument()
    })
  })
})
