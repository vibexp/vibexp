import { render, screen } from '@testing-library/react'

import type { TeamMember } from '@/services/teamService'

import {
  FeedActorAvatar,
  resolveFeedActor,
  USER_POST_ASSISTANT_NAME,
} from '../feedActor'

const member: TeamMember = {
  user_id: 'u-1',
  email: 'alice@example.com',
  name: 'Alice Smith',
  role: 'member',
  joined_at: new Date().toISOString(),
}

describe('resolveFeedActor', () => {
  it('classifies AI when ai_assistant_name is set and not the user-post sentinel', () => {
    const actor = resolveFeedActor(
      { ai_assistant_name: 'claude', posted_by_user_id: 'u-1' },
      undefined
    )
    expect(actor.isAi).toBe(true)
    expect(actor.displayName).toBe('claude')
    expect(actor.aiAssistantName).toBe('claude')
  })

  it.each(['User Post', 'user post', 'USER POST', '  User Post  '])(
    'classifies %s as a user post (case- and whitespace-insensitive sentinel)',
    name => {
      const actor = resolveFeedActor(
        { ai_assistant_name: name, posted_by_user_id: 'u-1' },
        member
      )
      expect(actor.isAi).toBe(false)
      expect(actor.displayName).toBe('Alice Smith')
    }
  )

  it('classifies as user when ai_assistant_name equals the user-post sentinel', () => {
    const actor = resolveFeedActor(
      {
        ai_assistant_name: USER_POST_ASSISTANT_NAME,
        posted_by_user_id: 'u-1',
      },
      member
    )
    expect(actor.isAi).toBe(false)
    expect(actor.displayName).toBe('Alice Smith')
    expect(actor.member).toBe(member)
  })

  it('classifies as user when ai_assistant_name is null', () => {
    const actor = resolveFeedActor(
      { ai_assistant_name: null, posted_by_user_id: 'u-1' },
      member
    )
    expect(actor.isAi).toBe(false)
    expect(actor.displayName).toBe('Alice Smith')
  })

  it('classifies as user when ai_assistant_name is empty string', () => {
    const actor = resolveFeedActor(
      { ai_assistant_name: '   ', posted_by_user_id: 'u-1' },
      member
    )
    expect(actor.isAi).toBe(false)
    expect(actor.displayName).toBe('Alice Smith')
  })

  it('falls back to "Unknown user" when no member is supplied', () => {
    const actor = resolveFeedActor(
      { ai_assistant_name: null, posted_by_user_id: 'unknown' },
      undefined
    )
    expect(actor.displayName).toBe('Unknown user')
    expect(actor.member).toBeUndefined()
  })
})

describe('FeedActorAvatar', () => {
  it('renders a provider icon for an AI actor', () => {
    const actor = resolveFeedActor({ ai_assistant_name: 'claude' }, undefined)
    render(<FeedActorAvatar actor={actor} />)
    expect(screen.getByTestId('ai-assistant-icon-claude')).toBeInTheDocument()
    expect(screen.getByAltText(/claude logo/i)).toBeInTheDocument()
  })

  it('renders the initial of the resolved member for a user actor', () => {
    const actor = resolveFeedActor(
      { ai_assistant_name: USER_POST_ASSISTANT_NAME },
      member
    )
    render(<FeedActorAvatar actor={actor} />)
    expect(screen.getByText('A')).toBeInTheDocument()
  })

  it('renders "?" when the user actor has no resolved member', () => {
    const actor = resolveFeedActor({ ai_assistant_name: null }, undefined)
    render(<FeedActorAvatar actor={actor} />)
    expect(screen.getByText('?')).toBeInTheDocument()
  })
})
