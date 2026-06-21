import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { RelativeTime } from '../RelativeTime'

// Radix Tooltip (via popper) relies on ResizeObserver, which jsdom lacks.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
})

describe('RelativeTime', () => {
  it('renders a relative label for a recent date', () => {
    jest.useFakeTimers()
    jest.setSystemTime(new Date('2024-06-01T12:00:00Z'))
    const recent = new Date('2024-06-01T09:00:00Z').toISOString()

    render(<RelativeTime value={recent} />)

    expect(screen.getByText('3h ago')).toBeInTheDocument()
    jest.useRealTimers()
  })

  it('renders a short date for dates older than a week', () => {
    // No fake timers: a 2024 date is always older than a week → short date.
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    const label = screen.getByText(/Jan 15, 2024/)
    expect(label).toBeInTheDocument()
    // The compact label must NOT carry the old redundant parenthetical suffix.
    expect(label.textContent).not.toMatch(/\(.*\)/)
  })

  it('reveals the full date-time on hover via tooltip', async () => {
    const user = userEvent.setup()
    render(<RelativeTime value="2024-01-15T12:00:00Z" />)

    // Before hover, only the compact label is in the document.
    expect(screen.queryByText(/January 15, 2024/)).not.toBeInTheDocument()

    await user.hover(screen.getByText(/Jan 15, 2024/))

    // Radix renders the tooltip content (with a visually-hidden a11y copy).
    const full = await screen.findAllByText(/January 15, 2024/)
    expect(full.length).toBeGreaterThan(0)
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
})
