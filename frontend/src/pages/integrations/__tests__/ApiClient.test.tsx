import { fireEvent, render, screen } from '@testing-library/react'

import { ApiClient } from '../ApiClient'

describe('API Client integration page', () => {
  it('renders without any team or auth provider', () => {
    render(<ApiClient />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'API Client' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Go' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { level: 2, name: 'TypeScript / JavaScript' })
    ).toBeInTheDocument()
  })

  it('shows the install command for each client', () => {
    const { container } = render(<ApiClient />)

    // CodeBlock highlights tokens into spans, so read each block's full text.
    const snippets = Array.from(container.querySelectorAll('pre code')).map(
      el => el.textContent
    )
    expect(snippets).toEqual([
      'go get github.com/vibexp/api-client-go@latest',
      'npm install @vibexp/api-client',
    ])
  })

  it('links out to each client repository for full documentation', () => {
    render(<ApiClient />)

    const goLink = screen.getByRole('link', {
      name: /vibexp\/api-client-go on github/i,
    })
    expect(goLink).toHaveAttribute(
      'href',
      'https://github.com/vibexp/api-client-go'
    )
    expect(goLink).toHaveAttribute('target', '_blank')
    expect(goLink).toHaveAttribute('rel', 'noopener noreferrer')

    const jsLink = screen.getByRole('link', {
      name: /vibexp\/api-client-js on github/i,
    })
    expect(jsLink).toHaveAttribute(
      'href',
      'https://github.com/vibexp/api-client-js'
    )
    expect(jsLink).toHaveAttribute('target', '_blank')
    expect(jsLink).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('copies an install command to the clipboard', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    render(<ApiClient />)

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[1])

    expect(writeText).toHaveBeenCalledWith('npm install @vibexp/api-client')
  })
})
