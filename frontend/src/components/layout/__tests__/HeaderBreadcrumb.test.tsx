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
