import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { DeferredToolPage } from '../DeferredToolPage'

describe('DeferredToolPage', () => {
  it('renders the deferred shell with a back link to the tool overview', () => {
    render(
      <MemoryRouter>
        <DeferredToolPage
          title="Claude Code Sessions"
          description="Session history is coming in a later slice."
          backHref="/ai-tools/claude-code/overview"
        />
      </MemoryRouter>
    )

    expect(screen.getByText('Claude Code Sessions')).toBeInTheDocument()
    expect(
      screen.getByText('Session history is coming in a later slice.')
    ).toBeInTheDocument()
    expect(screen.getByText('Coming soon')).toBeInTheDocument()

    const backLink = screen.getByRole('link', { name: /Back/ })
    expect(backLink).toHaveAttribute('href', '/ai-tools/claude-code/overview')
  })
})
