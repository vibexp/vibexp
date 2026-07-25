import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { HeaderBreadcrumb } from '../HeaderBreadcrumb'

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <HeaderBreadcrumb />
    </MemoryRouter>
  )
}

describe('HeaderBreadcrumb', () => {
  it('always renders the "VibeXP" root crumb', () => {
    renderAt('/')
    expect(screen.getByText('VibeXP')).toBeInTheDocument()
  })

  it('resolves the home route to "Dashboard"', () => {
    renderAt('/')
    expect(screen.getByText('Dashboard')).toBeInTheDocument()
  })

  it('resolves a leaf route to its nav label', () => {
    renderAt('/feeds')
    expect(screen.getByText('AI Feeds')).toBeInTheDocument()
  })

  it('prefers the longest matching href (deep child over parent)', () => {
    // `/ai-tools/claude-code/overview` matches both the parent
    // (`/ai-tools/overview` does not prefix it) and the child exactly.
    renderAt('/ai-tools/claude-code/overview')
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
  })

  it('matches nested sub-routes via the parent prefix', () => {
    renderAt('/settings/api-keys')
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  // #538 moved the team pages to top-level `/teams/**`. The nav entry is what
  // makes the breadcrumb resolve, so assert all three routes rather than
  // trusting the longest-prefix match to cover the nested ones.
  it.each(['/teams', '/teams/team-1', '/teams/team-1/analytics'])(
    'resolves %s to "Teams"',
    path => {
      renderAt(path)
      expect(screen.getByText('Teams')).toBeInTheDocument()
    }
  )

  it('no longer resolves the retired /settings/teams path to "Teams"', () => {
    // The route is deleted, not redirected (epic #536 decision 9) - it falls
    // back to the `/settings` prefix, which is what NotFound sits under.
    renderAt('/settings/teams')
    expect(screen.getByText('Settings')).toBeInTheDocument()
    expect(screen.queryByText('Teams')).not.toBeInTheDocument()
  })

  it('does not mistake a sibling prefix for a match', () => {
    // `/prompt-gallery` must resolve to its own entry, not "Prompts"
    // (`/prompts`), which only differs by a suffix.
    renderAt('/prompt-gallery')
    expect(screen.getByText('Prompt Gallery')).toBeInTheDocument()
    expect(screen.queryByText('Prompts')).not.toBeInTheDocument()
  })

  it('renders only the root crumb for an unknown route', () => {
    renderAt('/totally-unknown')
    expect(screen.getByText('VibeXP')).toBeInTheDocument()
    // No page segment and no divider when nothing matches.
    expect(screen.queryByText('/')).not.toBeInTheDocument()
  })
})
