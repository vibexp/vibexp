import { fireEvent, render, screen } from '@testing-library/react'

import { CLI } from '../CLI'

describe('CLI integration page', () => {
  it('renders without any team or auth provider', () => {
    render(<CLI />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'VibeXP CLI' })
    ).toBeInTheDocument()
  })

  it('shows the install commands from the vibexp/cli README', () => {
    const { container } = render(<CLI />)

    // CodeBlock highlights tokens into spans, so read each block's full text.
    const snippets = Array.from(container.querySelectorAll('pre code')).map(
      el => el.textContent
    )
    expect(snippets).toEqual([
      'brew install vibexp/tap/vibexp',
      'go install github.com/vibexp/cli/cmd/vibexp@latest',
    ])
    expect(
      screen.getByRole('link', { name: 'latest release' })
    ).toHaveAttribute('href', 'https://github.com/vibexp/cli/releases/latest')
  })

  it('links out to the CLI repository for full documentation', () => {
    render(<CLI />)

    const link = screen.getByRole('link', {
      name: /full documentation on github/i,
    })
    expect(link).toHaveAttribute('href', 'https://github.com/vibexp/cli')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('mentions the vibexp api escape hatch', () => {
    render(<CLI />)

    expect(screen.getByText('vibexp api')).toBeInTheDocument()
  })

  it('copies an install command to the clipboard', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    render(<CLI />)

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0])

    expect(writeText).toHaveBeenCalledWith('brew install vibexp/tap/vibexp')
  })
})
