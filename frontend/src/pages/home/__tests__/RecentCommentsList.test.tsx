import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { RecentCommentsList } from '@/pages/home/RecentLists'
import type { RecentComment } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'

function makeRecent(overrides: Partial<RecentComment> = {}): RecentComment {
  return {
    user_id: 'user-1',
    created_at: '2026-07-16T09:00:00Z',
    updated_at: '2026-07-16T09:00:00Z',
    resource_type: 'artifact',
    resource_id: 'res-1',
    resource_title: 'Q3 analysis',
    project_id: 'proj-1',
    slug: 'q3-analysis',
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
      role: 'member',
      joined_at: '2026-01-01T00:00:00Z',
    },
  ],
])

function renderList(comments: RecentComment[]) {
  return render(
    <MemoryRouter>
      <RecentCommentsList
        comments={comments}
        members={members}
        loading={false}
        error={null}
      />
    </MemoryRouter>
  )
}

describe('RecentCommentsList', () => {
  it('renders the empty state when there are no comments', () => {
    renderList([])
    expect(
      screen.getByText('No comments yet in this team.')
    ).toBeInTheDocument()
  })

  it('shows loading skeletons and no empty state while loading', () => {
    render(
      <MemoryRouter>
        <RecentCommentsList
          comments={[]}
          members={members}
          loading
          error={null}
        />
      </MemoryRouter>
    )
    expect(
      screen.queryByText('No comments yet in this team.')
    ).not.toBeInTheDocument()
  })

  it('renders author, "commented on" vs "edited a comment on", and resource title', () => {
    renderList([
      makeRecent({ resource_id: 'res-1', resource_title: 'Q3 analysis' }),
      makeRecent({
        resource_id: 'res-2',
        resource_title: 'Onboarding',
        updated_at: '2026-07-16T10:00:00Z', // edited (> created_at)
      }),
    ])
    expect(screen.getAllByText('Alice')).toHaveLength(2)
    expect(screen.getByText('commented on')).toBeInTheDocument()
    expect(screen.getByText('edited a comment on')).toBeInTheDocument()
    expect(screen.getByText('Q3 analysis')).toBeInTheDocument()
    expect(screen.getByText('Onboarding')).toBeInTheDocument()
  })

  it('links each row to the correct detail page for all four resource types', () => {
    renderList([
      makeRecent({
        resource_type: 'artifact',
        resource_id: 'a1',
        slug: 'art',
        project_id: 'proj-1',
      }),
      makeRecent({
        resource_type: 'blueprint',
        resource_id: 'b1',
        slug: 'bp',
        project_id: 'proj-2',
      }),
      makeRecent({
        resource_type: 'prompt',
        resource_id: 'p1',
        slug: 'pr',
        project_id: 'proj-3',
      }),
      makeRecent({
        resource_type: 'memory',
        resource_id: 'm1',
        slug: undefined,
        project_id: 'proj-4',
      }),
    ])
    const hrefs = screen.getAllByRole('link').map(l => l.getAttribute('href'))
    expect(hrefs).toEqual([
      '/artifacts/proj-1/art',
      '/blueprints/proj-2/bp',
      '/prompts/pr',
      '/memories/m1',
    ])
  })
})
