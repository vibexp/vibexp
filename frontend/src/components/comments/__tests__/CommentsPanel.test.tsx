import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { UseCommentsResult } from '@/hooks/useComments'
import type { Comment } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'

import { CommentsPanel } from '../CommentsPanel'

// Radix DropdownMenu / AlertDialog need these shims in jsdom.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

let mockState: UseCommentsResult
jest.mock('@/hooks/useComments', () => ({
  useComments: () => mockState,
}))

let mockCanCreate = true
let mockCanDelete = false
jest.mock('@/hooks/usePermissions', () => ({
  usePermissions: () => ({
    can: (p: string) => (p === 'resource.create' ? mockCanCreate : false),
    canDeleteResource: () => mockCanDelete,
    canDeleteFeedContent: () => false,
  }),
}))

let mockUserId: string | undefined = 'user-1'
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: mockUserId ? { id: mockUserId } : null }),
}))

const showError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showError }),
}))

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'c1',
    team_id: 'team-1',
    resource_type: 'artifact',
    resource_id: 'res-1',
    user_id: 'user-1',
    content: 'A comment body',
    created_at: '2026-07-16T09:00:00Z',
    updated_at: '2026-07-16T09:00:00Z',
    ...overrides,
  }
}

const members = new Map<string, TeamMember>([
  [
    'user-1',
    {
      user_id: 'user-1',
      email: 'a@x.com',
      name: 'Alice',
      role: 'owner',
      joined_at: '2026-01-01T00:00:00Z',
    },
  ],
  [
    'user-2',
    {
      user_id: 'user-2',
      email: 'b@x.com',
      name: 'Bob',
      role: 'member',
      joined_at: '2026-01-01T00:00:00Z',
    },
  ],
])

function makeState(over: Partial<UseCommentsResult> = {}): UseCommentsResult {
  return {
    comments: [],
    members,
    totalCount: 0,
    loading: false,
    loadingMore: false,
    error: false,
    hasMore: false,
    reload: jest.fn(),
    loadMore: jest.fn(),
    addComment: jest.fn().mockResolvedValue(undefined),
    editComment: jest.fn().mockResolvedValue(undefined),
    removeComment: jest.fn().mockResolvedValue(undefined),
    ...over,
  }
}

function renderPanel() {
  return render(
    <CommentsPanel teamId="team-1" resourceType="artifact" resourceId="res-1" />
  )
}

beforeEach(() => {
  jest.clearAllMocks()
  mockCanCreate = true
  mockCanDelete = false
  mockUserId = 'user-1'
})

describe('CommentsPanel', () => {
  it('shows loading skeletons while the initial load is in flight', () => {
    mockState = makeState({ loading: true })
    renderPanel()
    expect(screen.getByTestId('comments-loading')).toBeInTheDocument()
  })

  it('shows an error state with a working Retry', async () => {
    const reload = jest.fn()
    mockState = makeState({ error: true, reload })
    renderPanel()

    expect(screen.getByText(/couldn't load comments/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(reload).toHaveBeenCalled()
  })

  it('shows the empty state when there are no comments', () => {
    mockState = makeState()
    renderPanel()
    expect(screen.getByText(/no comments yet/i)).toBeInTheDocument()
  })

  it('renders the comment rows', () => {
    mockState = makeState({
      comments: [
        makeComment({ id: 'a' }),
        makeComment({ id: 'b', user_id: 'user-2' }),
      ],
      totalCount: 2,
    })
    renderPanel()
    expect(screen.getAllByTestId('comment-row')).toHaveLength(2)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('Bob')).toBeInTheDocument()
  })

  it('hides the footer at N <= 5 and shows it (with the count) beyond', () => {
    mockState = makeState({ comments: [makeComment()], totalCount: 5 })
    const { rerender } = renderPanel()
    expect(screen.queryByTestId('comments-see-all')).not.toBeInTheDocument()

    mockState = makeState({ comments: [makeComment()], totalCount: 8 })
    rerender(
      <CommentsPanel
        teamId="team-1"
        resourceType="artifact"
        resourceId="res-1"
      />
    )
    const footer = screen.getByTestId('comments-see-all')
    expect(footer).toHaveTextContent('8')
  })

  it('opens the all-comments popup from the footer', async () => {
    mockState = makeState({ comments: [makeComment()], totalCount: 8 })
    renderPanel()
    await userEvent.click(screen.getByTestId('comments-see-all'))
    expect(
      screen.getByRole('dialog', { name: /comments \(8\)/i })
    ).toBeInTheDocument()
  })

  describe('action visibility (permission matrix)', () => {
    it('shows the actions menu on the user’s own comment', () => {
      mockState = makeState({
        comments: [makeComment({ user_id: 'user-1' })],
        totalCount: 1,
      })
      renderPanel()
      expect(
        screen.getByRole('button', { name: 'Comment actions' })
      ).toBeInTheDocument()
    })

    it('hides the menu on another member’s comment for a plain member', () => {
      mockCanDelete = false
      mockState = makeState({
        comments: [makeComment({ user_id: 'user-2' })],
        totalCount: 1,
      })
      renderPanel()
      expect(
        screen.queryByRole('button', { name: 'Comment actions' })
      ).not.toBeInTheDocument()
    })

    it('shows the menu on another member’s comment for an admin/owner (delete-any)', () => {
      mockCanDelete = true
      mockState = makeState({
        comments: [makeComment({ user_id: 'user-2' })],
        totalCount: 1,
      })
      renderPanel()
      expect(
        screen.getByRole('button', { name: 'Comment actions' })
      ).toBeInTheDocument()
    })
  })

  it('hides the add affordance when the user cannot create comments', () => {
    mockCanCreate = false
    mockState = makeState()
    renderPanel()
    expect(screen.queryByTestId('comment-add-button')).not.toBeInTheDocument()
  })

  it('adds a comment from the inline composer', async () => {
    const user = userEvent.setup()
    const addComment = jest.fn().mockResolvedValue(undefined)
    mockState = makeState({ addComment })
    renderPanel()

    await user.click(screen.getByTestId('comment-add-button'))
    await user.type(screen.getByRole('textbox'), 'nice work')
    await user.click(screen.getByRole('button', { name: 'Comment' }))

    expect(addComment).toHaveBeenCalledWith('nice work')
  })

  it('deletes a comment through the confirm dialog', async () => {
    const user = userEvent.setup()
    mockCanDelete = true
    const removeComment = jest.fn().mockResolvedValue(undefined)
    mockState = makeState({
      comments: [makeComment({ id: 'a', user_id: 'user-1' })],
      totalCount: 1,
      removeComment,
    })
    renderPanel()

    await user.click(screen.getByRole('button', { name: 'Comment actions' }))
    await user.click(await screen.findByRole('menuitem', { name: /delete/i }))

    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(removeComment).toHaveBeenCalledWith('a')
    })
  })
})
