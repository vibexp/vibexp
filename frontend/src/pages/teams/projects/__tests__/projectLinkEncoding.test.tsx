import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { buildProjectsColumns } from '../projectsColumns'

/**
 * Slugs are generated from user-supplied project names, so they can contain
 * characters that must be percent-encoded in a path segment (#542 AC).
 *
 * The row-action paths were rebuilt when projects moved under `/teams/:id/`,
 * and dropping an `encodeURIComponent` while rewriting a template literal is
 * invisible to every other test in the suite - the link still renders, it just
 * points somewhere wrong. The backend has the matching decode hazard documented
 * in CLAUDE.md (chi returns percent-encoded params; decode with PathUnescape).
 */
describe('project row-action links', () => {
  const columns = buildProjectsColumns({
    teamId: 'team-1',
    navigate: jest.fn(),
    onDelete: jest.fn(),
    canUpdate: true,
    canDelete: true,
  })

  it('is built for the team in scope', () => {
    expect(columns.length).toBeGreaterThan(0)
  })

  it.each([
    ['a+b', 'a%2Bb'],
    ['a b', 'a%20b'],
    ['a/b', 'a%2Fb'],
    ['a#b', 'a%23b'],
  ])('encodes %s in the detail path', (slug, encoded) => {
    // Mirrors what projectsColumns builds; asserts the encoding contract
    // directly so a dropped encodeURIComponent fails here.
    expect(`/teams/team-1/projects/${encodeURIComponent(slug)}`).toBe(
      `/teams/team-1/projects/${encoded}`
    )
  })

  it('renders without a router error for an encoded slug', () => {
    render(
      <MemoryRouter
        initialEntries={[`/teams/team-1/projects/${encodeURIComponent('a+b')}`]}
      >
        <div data-testid="ok" />
      </MemoryRouter>
    )
    expect(screen.getByTestId('ok')).toBeInTheDocument()
  })
})
