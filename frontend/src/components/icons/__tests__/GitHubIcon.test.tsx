import { render } from '@testing-library/react'

import { GitHubIcon } from '../GitHubIcon'

describe('GitHubIcon', () => {
  it('renders a decorative svg hidden from assistive technology', () => {
    const { container } = render(<GitHubIcon />)
    const svg = container.querySelector('svg')

    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg).toHaveAttribute('focusable', 'false')
  })

  it('applies the given className', () => {
    const { container } = render(<GitHubIcon className="h-4 w-4" />)

    expect(container.querySelector('svg')).toHaveClass('h-4', 'w-4')
  })
})
