import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ResourceHeaderMeta } from '../ResourceHeaderMeta'

describe('ResourceHeaderMeta', () => {
  it('renders nothing visible when every prop is absent', () => {
    const { container } = render(<ResourceHeaderMeta />)

    expect(screen.getByTestId('resource-header-meta')).toBeEmptyDOMElement()
    expect(container.querySelector('button')).toBeNull()
  })

  it('renders the status badge with its tone', () => {
    render(<ResourceHeaderMeta status={{ value: 'Active', tone: 'success' }} />)

    const badge = screen.getByText('Active')
    expect(badge).toBeInTheDocument()
    expect(badge.classList.contains('bg-success')).toBe(true)
  })

  it('renders the address as a copy chip that names the value', () => {
    render(<ResourceHeaderMeta address={{ value: 'code-review-template' }} />)

    expect(
      screen.getByRole('button', { name: 'Copy slug: code-review-template' })
    ).toBeInTheDocument()
  })

  it('honours a custom address label', () => {
    render(<ResourceHeaderMeta address={{ label: 'ID', value: 'abc123' }} />)

    expect(
      screen.getByRole('button', { name: 'Copy id: abc123' })
    ).toBeInTheDocument()
  })

  it('renders a non-copyable address as a plain chip, not a button', () => {
    render(
      <ResourceHeaderMeta address={{ value: 'read-only', copyable: false }} />
    )

    expect(screen.getByText('read-only')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders the updated time as a relative label', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString()

    render(<ResourceHeaderMeta updatedAt={twoHoursAgo} />)

    expect(screen.getByText('Updated')).toBeInTheDocument()
    expect(screen.getByText('2h ago')).toBeInTheDocument()
  })

  it('renders the summary on its own line', () => {
    render(<ResourceHeaderMeta summary="A reusable review checklist" />)

    expect(screen.getByText('A reusable review checklist')).toBeInTheDocument()
  })

  it('renders kind-specific extras beside the status', () => {
    render(
      <ResourceHeaderMeta
        status={{ value: 'published', tone: 'success' }}
        extra={<span>Shared</span>}
      />
    )

    expect(screen.getByText('published')).toBeInTheDocument()
    expect(screen.getByText('Shared')).toBeInTheDocument()
  })

  it('renders every part together, status first and updated last', () => {
    render(
      <ResourceHeaderMeta
        status={{ value: 'Active', tone: 'success' }}
        address={{ value: 'my-artifact' }}
        updatedAt={new Date(Date.now() - 3 * 60 * 1000).toISOString()}
        summary="An artifact summary"
      />
    )

    const row = screen.getByTestId('resource-header-meta')
    const text = row.textContent
    expect(text.indexOf('Active')).toBeLessThan(text.indexOf('my-artifact'))
    expect(text.indexOf('my-artifact')).toBeLessThan(text.indexOf('Updated'))
    expect(text.indexOf('Updated')).toBeLessThan(
      text.indexOf('An artifact summary')
    )
  })

  it('copies the address to the clipboard when the chip is clicked', async () => {
    const user = userEvent.setup()
    render(<ResourceHeaderMeta address={{ value: 'copy-me' }} />)

    await user.click(screen.getByRole('button', { name: /Copy slug/ }))

    await expect(navigator.clipboard.readText()).resolves.toBe('copy-me')
  })
})
