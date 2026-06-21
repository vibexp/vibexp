import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import type { FeedItemReply, FeedItemReplyListResponse } from '@/types/feed'
import type { TeamMember } from '@/types/team'

import { FeedItemReplies } from '../FeedItemReplies'

// Mock MarkdownRenderer to render plain text in tests
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({
    content,
    className,
  }: {
    content: string
    className?: string
  }) => <div className={className}>{content}</div>,
}))

// Mock feedService
jest.mock('@/services/feedService', () => ({
  feedService: {
    listReplies: jest.fn(),
    createReply: jest.fn(),
  },
}))

// Mock teamService
jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamMembers: jest.fn(),
  },
}))

// Stable mock for useErrorHandler to avoid AlertContext dependency and
// prevent infinite re-renders caused by a new jest.fn() reference on each render.
const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({
    handleError: mockHandleError,
  }),
}))

const { feedService } = require('@/services/feedService') as {
  feedService: {
    listReplies: jest.Mock
    createReply: jest.Mock
  }
}

const { teamService } = require('@/services/teamService') as {
  teamService: {
    getTeamMembers: jest.Mock
  }
}

const emptyResponse: FeedItemReplyListResponse = {
  replies: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const teamMember: TeamMember = {
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice Smith',
  role: 'member',
  joined_at: new Date().toISOString(),
}

const humanReply: FeedItemReply = {
  id: 'reply-1',
  team_id: 'team-1',
  feed_item_id: 'item-1',
  content: 'Great post!',
  posted_by_user_id: 'user-1',
  ai_assistant_name: null,
  posted_at: new Date(Date.now() - 60000).toISOString(),
}

const aiReply: FeedItemReply = {
  id: 'reply-2',
  team_id: 'team-1',
  feed_item_id: 'item-1',
  content: 'AI generated response.',
  posted_by_user_id: 'user-1',
  ai_assistant_name: 'claude-sonnet',
  posted_at: new Date(Date.now() - 120000).toISOString(),
}

const repliesResponse: FeedItemReplyListResponse = {
  replies: [humanReply, aiReply],
  total_count: 2,
  page: 1,
  per_page: 20,
  total_pages: 1,
}

function renderReplies() {
  return render(<FeedItemReplies teamId="team-1" itemId="item-1" />)
}

describe('FeedItemReplies', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    // Default: return empty team members list
    teamService.getTeamMembers.mockResolvedValue([])
  })

  it('renders the compose textarea after loading (compose box is at bottom)', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
        expect(
          screen.getByPlaceholderText('Write a reply...')
        ).toBeInTheDocument()
        expect(
          screen.getByRole('button', { name: 'Reply' })
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('Reply button is disabled when textarea is empty', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByPlaceholderText('Write a reply...')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    expect(screen.getByRole('button', { name: 'Reply' })).toBeDisabled()
  })

  it('Reply button is enabled when textarea has content', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByPlaceholderText('Write a reply...')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    fireEvent.change(screen.getByPlaceholderText('Write a reply...'), {
      target: { value: 'Hello world' },
    })
    expect(screen.getByRole('button', { name: 'Reply' })).not.toBeDisabled()
  })

  it('shows "No replies yet" when there are no replies', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('renders human reply with real member name when team member is found', async () => {
    feedService.listReplies.mockResolvedValue(repliesResponse)
    teamService.getTeamMembers.mockResolvedValue([teamMember])
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('Great post!')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    // Should show real name, not "Team member"
    expect(screen.getByText('Alice Smith')).toBeInTheDocument()
    expect(screen.queryByText('Team member')).not.toBeInTheDocument()
  })

  it('renders human reply with "Unknown user" when team member cannot be resolved', async () => {
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [humanReply],
      total_count: 1,
    })
    // No matching member in team
    teamService.getTeamMembers.mockResolvedValue([])
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('Great post!')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    expect(screen.getByText('Unknown user')).toBeInTheDocument()
  })

  it('renders AI reply with assistant name and AI badge', async () => {
    feedService.listReplies.mockResolvedValue(repliesResponse)
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('AI generated response.')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    expect(screen.getByText('claude-sonnet')).toBeInTheDocument()
    expect(screen.getByText('AI')).toBeInTheDocument()
  })

  it('renders reply timestamps as relative time', async () => {
    feedService.listReplies.mockResolvedValue(repliesResponse)
    renderReplies()
    await waitFor(
      () => {
        const timeElements = screen.getAllByText(/ago/)
        expect(timeElements.length).toBeGreaterThanOrEqual(1)
      },
      { timeout: 3000 }
    )
  })

  it('human reply avatar shows user initial when member is resolved', async () => {
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [humanReply],
      total_count: 1,
    })
    teamService.getTeamMembers.mockResolvedValue([teamMember])
    renderReplies()
    await waitFor(
      () => {
        // Alice Smith → initial 'A'
        expect(screen.getByText('A')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('unknown user avatar shows "?" initial', async () => {
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [humanReply],
      total_count: 1,
    })
    teamService.getTeamMembers.mockResolvedValue([])
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('?')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('AI reply avatar renders the matched provider icon (claude)', async () => {
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [aiReply],
      total_count: 1,
    })
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByTestId('ai-assistant-icon-claude')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    const icon = screen.getByAltText(/claude logo/i)
    expect(icon).toBeInTheDocument()
    expect(icon.tagName).toBe('IMG')
  })

  it('AI reply avatar renders openai icon for codex assistant', async () => {
    const codexReply: FeedItemReply = {
      ...aiReply,
      id: 'reply-codex',
      ai_assistant_name: 'Codex',
    }
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [codexReply],
      total_count: 1,
    })
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByTestId('ai-assistant-icon-openai')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('AI reply avatar renders gemini icon for google assistant', async () => {
    const googleReply: FeedItemReply = {
      ...aiReply,
      id: 'reply-google',
      ai_assistant_name: 'Google Bard',
    }
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [googleReply],
      total_count: 1,
    })
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByTestId('ai-assistant-icon-gemini')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('AI reply avatar falls back to generic icon for unknown assistant', async () => {
    const unknownReply: FeedItemReply = {
      ...aiReply,
      id: 'reply-unknown',
      ai_assistant_name: 'mistral-large',
    }
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [unknownReply],
      total_count: 1,
    })
    renderReplies()
    await waitFor(
      () => {
        expect(
          screen.getByTestId('ai-assistant-icon-generic')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
  })

  it('new replies are prepended at the top of the list (newest-first order)', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    const firstReply: FeedItemReply = {
      id: 'reply-first',
      team_id: 'team-1',
      feed_item_id: 'item-1',
      content: 'First reply text',
      posted_by_user_id: 'user-1',
      ai_assistant_name: null,
      posted_at: new Date(Date.now() - 5000).toISOString(),
    }
    const secondReply: FeedItemReply = {
      id: 'reply-second',
      team_id: 'team-1',
      feed_item_id: 'item-1',
      content: 'Second reply text',
      posted_by_user_id: 'user-1',
      ai_assistant_name: null,
      posted_at: new Date().toISOString(),
    }
    feedService.createReply
      .mockResolvedValueOnce(firstReply)
      .mockResolvedValueOnce(secondReply)

    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    // Submit first reply
    fireEvent.change(screen.getByPlaceholderText('Write a reply...'), {
      target: { value: 'First reply text' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }))

    // Wait for first reply to appear and textarea to clear
    await waitFor(
      () => {
        expect(screen.getByText('First reply text')).toBeInTheDocument()
        const ta = screen.getByPlaceholderText('Write a reply...')
        expect((ta as HTMLTextAreaElement).value).toBe('')
      },
      { timeout: 3000 }
    )

    // Submit second reply
    fireEvent.change(screen.getByPlaceholderText('Write a reply...'), {
      target: { value: 'Second reply text' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }))

    await waitFor(
      () => {
        expect(screen.getByText('Second reply text')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    // Verify the second (newest) reply appears BEFORE the first in DOM order
    // matches backend ORDER BY posted_at DESC (newest first)
    const firstEl = screen.getByText('First reply text')
    const secondEl = screen.getByText('Second reply text')
    // compareDocumentPosition: 2 means 'preceding' (second comes before first)
    const position = firstEl.compareDocumentPosition(secondEl)
    expect(position & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy()
  })

  it('compose box renders after the replies list', async () => {
    feedService.listReplies.mockResolvedValue({
      ...emptyResponse,
      replies: [humanReply],
      total_count: 1,
    })
    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('Great post!')).toBeInTheDocument()
        expect(
          screen.getByPlaceholderText('Write a reply...')
        ).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    // The compose textarea should appear in the DOM after the reply content
    const replyContent = screen.getByText('Great post!')
    const textarea = screen.getByPlaceholderText('Write a reply...')
    // compareDocumentPosition: 4 means 'following' (textarea comes after reply)
    const position = replyContent.compareDocumentPosition(textarea)
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('submits reply and prepends it to the top of the list', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    const newReply: FeedItemReply = {
      id: 'reply-new',
      team_id: 'team-1',
      feed_item_id: 'item-1',
      content: 'New reply text',
      posted_by_user_id: 'user-1',
      ai_assistant_name: null,
      posted_at: new Date().toISOString(),
    }
    feedService.createReply.mockResolvedValue(newReply)

    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    fireEvent.change(screen.getByPlaceholderText('Write a reply...'), {
      target: { value: 'New reply text' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }))

    await waitFor(
      () => {
        expect(screen.getByText('New reply text')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    expect(feedService.createReply).toHaveBeenCalledWith('team-1', 'item-1', {
      content: 'New reply text',
    })
  })

  it('clears textarea after successful submit', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    const newReply: FeedItemReply = {
      id: 'reply-clear',
      team_id: 'team-1',
      feed_item_id: 'item-1',
      content: 'Another reply',
      posted_by_user_id: 'user-1',
      ai_assistant_name: null,
      posted_at: new Date().toISOString(),
    }
    feedService.createReply.mockResolvedValue(newReply)

    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    const textarea = screen.getByPlaceholderText('Write a reply...')
    fireEvent.change(textarea, { target: { value: 'Another reply' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }))

    await waitFor(
      () => {
        const textareaEl = screen.getByPlaceholderText('Write a reply...')
        expect((textareaEl as HTMLTextAreaElement).value).toBe('')
      },
      { timeout: 3000 }
    )
  })

  it('submits reply on Enter key (without Shift)', async () => {
    feedService.listReplies.mockResolvedValue(emptyResponse)
    const newReply: FeedItemReply = {
      id: 'reply-enter',
      team_id: 'team-1',
      feed_item_id: 'item-1',
      content: 'Keyboard submit',
      posted_by_user_id: 'user-1',
      ai_assistant_name: null,
      posted_at: new Date().toISOString(),
    }
    feedService.createReply.mockResolvedValue(newReply)

    renderReplies()
    await waitFor(
      () => {
        expect(screen.getByText('No replies yet')).toBeInTheDocument()
      },
      { timeout: 3000 }
    )

    const textarea = screen.getByPlaceholderText('Write a reply...')
    fireEvent.change(textarea, { target: { value: 'Keyboard submit' } })
    fireEvent.keyDown(textarea, { key: 'Enter', shiftKey: false })

    await waitFor(
      () => {
        expect(feedService.createReply).toHaveBeenCalledWith(
          'team-1',
          'item-1',
          { content: 'Keyboard submit' }
        )
      },
      { timeout: 3000 }
    )
  })
})
