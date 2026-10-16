import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { RelativeTime } from '../RelativeTime'

describe('RelativeTime', () => {
  it('renders a relative label for a recent date', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2024-06-01T12:00:00Z'))
    const recent = new Date('2024-06-01T09:00:00Z').toISOString()

    render(<RelativeTime value={recent} />)

    expect(screen.getByText('3h ago')).toBeInTheDocument()
    vi.useRealTimers()
  })

  it('renders a short date for dates older than a week', () => {
    // No fake timers: a 2024 date is always older than a week → short date.
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    const label = screen.getByText(/Jan 15, 2024/)
    expect(label).toBeInTheDocument()
    // The compact label must NOT carry the old redundant parenthetical suffix.
    expect(label.textContent).not.toMatch(/\(.*\)/)
  })

  it('does not render the full date-time as a second bubble on hover', async () => {
    // The absolute value is an attribute, not text: a JS tooltip alongside the
    // native `title` shows two bubbles on the same hover (#907). The assertion
    // has to come AFTER the hover — a Radix tooltip renders nothing until it
    // opens, so checking before would pass with the tooltip still there.
    const user = userEvent.setup()
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    await user.hover(screen.getByText(/Jan 15, 2024/))

    expect(screen.queryByText(/January 15, 2024/)).not.toBeInTheDocument()
  })

  it('exposes the absolute date-time as a `title` attribute', () => {
    // List cells are read by screen readers, copied, and asserted on in e2e —
    // all of which reach a `title` and none of which reach a JS tooltip (#907).
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    expect(screen.getByText(/Jan 15, 2024/)).toHaveAttribute(
      'title',
      expect.stringContaining('January 15, 2024')
    )
  })

  it('is a <time> element carrying the machine-readable instant', () => {
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    const label = screen.getByText(/Jan 15, 2024/)
    expect(label.tagName).toBe('TIME')
    expect(label).toHaveAttribute('dateTime', '2024-01-15T12:00:00.000Z')
  })

  it('omits `title` when there is no timestamp, rather than repeating "Never"', () => {
    render(<RelativeTime value={null} />)

    const label = screen.getByText('Never')
    expect(label).not.toHaveAttribute('title')
    // "Never" is not a time, so it must not claim to be one.
    expect(label.tagName).toBe('SPAN')
  })

  it('applies the provided className to the compact label', () => {
    render(
      <RelativeTime
        value="2024-01-15T12:00:00Z"
        className="text-muted-foreground text-sm"
      />
    )

    const label = screen.getByText(/Jan 15, 2024/)
    expect(label).toHaveClass('text-muted-foreground', 'text-sm')
  })

  it('formats the hover title with formatTitle when given', () => {
    render(
      <RelativeTime
        value="2024-01-15T12:00:00Z"
        formatTitle={date => `UTC ${date.toISOString()}`}
      />
    )

    expect(screen.getByText(/Jan 15, 2024/)).toHaveAttribute(
      'title',
      'UTC 2024-01-15T12:00:00.000Z'
    )
  })
})
