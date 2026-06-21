import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import type { FeedItem } from '@/types/feed'

import { USER_POST_ASSISTANT_NAME } from '../FeedPostComposer'
import { FeedPostComposer } from '../FeedPostComposer'

const mockHandleError = jest.fn()
jest.mock('@/services/feedService', () => ({
  feedService: { createFeedItem: jest.fn() },
}))
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1' } }),
}))
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({
    user: { id: 'user-1', name: 'Test User', email: 'test@example.com' },
  }),
}))
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))

const { feedService } = require('@/services/feedService') as {
  feedService: {
    createFeedItem: jest.Mock
  }
}

const mockFeedItem: FeedItem = {
  id: 'item-1',
  team_id: 'team-1',
  feed_id: 'feed-1',
  title: 'Test title',
  content: 'Test content',
  excerpt: 'Test content',
  ai_assistant_name: USER_POST_ASSISTANT_NAME,
  posted_by_user_id: 'user-1',
  posted_at: new Date().toISOString(),
}

function renderComposer(onPosted = jest.fn()) {
  const result = render(
    <FeedPostComposer feedId="feed-1" projects={[]} onPosted={onPosted} />
  )
  // Composer renders collapsed by default; focus the title input to expand
  // and reveal the content textarea + footer toolbar.
  fireEvent.focus(screen.getByRole('textbox', { name: /post title/i }))
  return result
}

describe('FeedPostComposer', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders title input, content textarea, and submit button', () => {
    renderComposer()

    expect(
      screen.getByRole('textbox', { name: /post title/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: /post content/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /post/i })).toBeInTheDocument()
  })

  it('submit button is disabled when title is empty', () => {
    renderComposer()

    const textarea = screen.getByRole('textbox', { name: /post content/i })
    fireEvent.change(textarea, { target: { value: 'Some content' } })

    expect(screen.getByRole('button', { name: /post/i })).toBeDisabled()
  })

  it('submit button is disabled when content is empty (title filled)', () => {
    renderComposer()

    const titleInput = screen.getByRole('textbox', { name: /post title/i })
    fireEvent.change(titleInput, { target: { value: 'A title' } })

    expect(screen.getByRole('button', { name: /post/i })).toBeDisabled()
  })

  it('submit button is disabled when both are whitespace-only', () => {
    renderComposer()

    const titleInput = screen.getByRole('textbox', { name: /post title/i })
    const textarea = screen.getByRole('textbox', { name: /post content/i })
    fireEvent.change(titleInput, { target: { value: '   ' } })
    fireEvent.change(textarea, { target: { value: '   ' } })

    expect(screen.getByRole('button', { name: /post/i })).toBeDisabled()
  })

  it('submit button is enabled when both title and content are non-empty', () => {
    renderComposer()

    const titleInput = screen.getByRole('textbox', { name: /post title/i })
    const textarea = screen.getByRole('textbox', { name: /post content/i })
    fireEvent.change(titleInput, { target: { value: 'A title' } })
    fireEvent.change(textarea, { target: { value: 'Some content' } })

    expect(screen.getByRole('button', { name: /post/i })).not.toBeDisabled()
  })

  it('successful submit calls feedService.createFeedItem with correct payload', async () => {
    feedService.createFeedItem.mockResolvedValue(mockFeedItem)
    renderComposer()

    fireEvent.change(screen.getByRole('textbox', { name: /post title/i }), {
      target: { value: 'My title' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: /post content/i }), {
      target: { value: 'My content' },
    })
    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    await waitFor(() => {
      expect(feedService.createFeedItem).toHaveBeenCalledWith(
        'team-1',
        'feed-1',
        {
          title: 'My title',
          content: 'My content',
          ai_assistant_name: USER_POST_ASSISTANT_NAME,
          project_id: undefined,
        }
      )
    })
  })

  it('does not reset form and does not call onPosted on submission failure', async () => {
    feedService.createFeedItem.mockRejectedValue(new Error('Network error'))
    const onPosted = jest.fn()
    render(
      <FeedPostComposer feedId="feed-1" projects={[]} onPosted={onPosted} />
    )

    const titleInput = screen.getByRole('textbox', { name: /post title/i })
    fireEvent.focus(titleInput)
    const textarea = screen.getByRole('textbox', { name: /post content/i })

    fireEvent.change(titleInput, { target: { value: 'My title' } })
    fireEvent.change(textarea, { target: { value: 'My content' } })
    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalled()
      expect((titleInput as HTMLInputElement).value).toBe('My title')
      expect((textarea as HTMLTextAreaElement).value).toBe('My content')
      expect(onPosted).not.toHaveBeenCalled()
    })
  })

  it('submit button re-enables after submission failure', async () => {
    feedService.createFeedItem.mockRejectedValue(new Error('Network error'))
    renderComposer()

    fireEvent.change(screen.getByRole('textbox', { name: /post title/i }), {
      target: { value: 'My title' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: /post content/i }), {
      target: { value: 'My content' },
    })
    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^post$/i })).not.toBeDisabled()
    })
  })

  it('form resets after successful submission', async () => {
    feedService.createFeedItem.mockResolvedValue(mockFeedItem)
    renderComposer()

    const titleInput = screen.getByRole('textbox', { name: /post title/i })
    fireEvent.change(titleInput, { target: { value: 'My title' } })
    fireEvent.change(screen.getByRole('textbox', { name: /post content/i }), {
      target: { value: 'My content' },
    })
    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    // After successful post the composer collapses; title input stays mounted
    // and is empty, textarea unmounts.
    await waitFor(() => {
      expect((titleInput as HTMLInputElement).value).toBe('')
      expect(
        screen.queryByRole('textbox', { name: /post content/i })
      ).not.toBeInTheDocument()
    })

    // Re-focus the title to re-expand: the new textarea should be empty.
    fireEvent.focus(titleInput)
    const reopened = screen.getByRole('textbox', { name: /post content/i })
    expect((reopened as HTMLTextAreaElement).value).toBe('')
  })

  it('submit button is disabled while submitting (loading state)', async () => {
    let resolveSubmit!: (value: FeedItem) => void
    feedService.createFeedItem.mockReturnValue(
      new Promise<FeedItem>(resolve => {
        resolveSubmit = resolve
      })
    )
    renderComposer()

    fireEvent.change(screen.getByRole('textbox', { name: /post title/i }), {
      target: { value: 'My title' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: /post content/i }), {
      target: { value: 'My content' },
    })

    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /posting/i })).toBeDisabled()
    })

    // Resolve to clean up
    resolveSubmit(mockFeedItem)
  })

  it('onPosted callback is called after successful submission', async () => {
    feedService.createFeedItem.mockResolvedValue(mockFeedItem)
    const onPosted = jest.fn()
    render(
      <FeedPostComposer feedId="feed-1" projects={[]} onPosted={onPosted} />
    )

    fireEvent.focus(screen.getByRole('textbox', { name: /post title/i }))
    fireEvent.change(screen.getByRole('textbox', { name: /post title/i }), {
      target: { value: 'My title' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: /post content/i }), {
      target: { value: 'My content' },
    })
    fireEvent.click(screen.getByRole('button', { name: /post/i }))

    await waitFor(() => {
      expect(onPosted).toHaveBeenCalledTimes(1)
    })
  })
})
