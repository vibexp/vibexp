import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'

import { MetadataPanel, MetaRow, MetaSlugRow } from '../MetadataPanel'

// MetadataPanel renders RelativeTime (Radix Tooltip), whose popper relies on
// ResizeObserver — jsdom doesn't provide it.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
})

const CREATED = '2024-01-01T00:00:00Z'
const UPDATED = '2024-01-02T00:00:00Z'

describe('MetaRow', () => {
  it('renders the label and children', () => {
    render(
      <ul>
        <MetaRow label="Type">general</MetaRow>
      </ul>
    )
    expect(screen.getByText('Type')).toBeInTheDocument()
    expect(screen.getByText('general')).toBeInTheDocument()
  })

  it('renders as a list item so it slots into the panel list', () => {
    render(
      <ul>
        <MetaRow label="Type">general</MetaRow>
      </ul>
    )
    expect(screen.getByRole('listitem')).toBeInTheDocument()
  })
})

describe('MetaSlugRow', () => {
  it('renders the value as a code chip that is itself the copy button', () => {
    render(
      <ul>
        <MetaSlugRow value="my-artifact-slug" />
      </ul>
    )
    const code = screen.getByText('my-artifact-slug')
    expect(code.tagName).toBe('CODE')
    const button = screen.getByRole('button', { name: /copy slug/i })
    expect(button).toBeInTheDocument()
    // The chip itself is the interactive control — the value lives inside it.
    expect(button).toContainElement(code)
    // aria-label overrides the inner <code>, so it must carry the value too,
    // otherwise assistive tech wouldn't announce what gets copied.
    expect(button).toHaveAccessibleName('Copy slug: my-artifact-slug')
  })

  it('truncates a long slug to a single line instead of wrapping', () => {
    const longSlug =
      'a-very-long-resource-slug-that-would-otherwise-wrap-' + 'x'.repeat(60)
    render(
      <ul>
        <MetaSlugRow value={longSlug} />
      </ul>
    )
    const code = screen.getByText(longSlug)
    // `truncate` (overflow-hidden + text-ellipsis + whitespace-nowrap) keeps the
    // chip on one line; `min-w-0` lets it shrink so the ellipsis can show. The
    // old wrapping behaviour (`break-all` + `flex-wrap`) must be gone.
    expect(code).toHaveClass('truncate', 'min-w-0')
    expect(code).not.toHaveClass('break-all')
    const button = screen.getByRole('button', { name: /copy slug/i })
    expect(button).not.toHaveClass('flex-wrap', 'break-all')
  })

  it('still copies the full, untruncated slug on click', async () => {
    const writeText = jest.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    const longSlug = 'full-' + 'y'.repeat(120)
    render(
      <ul>
        <MetaSlugRow value={longSlug} />
      </ul>
    )
    fireEvent.click(screen.getByRole('button', { name: /copy slug/i }))
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(longSlug)
    })
  })

  it('honours a custom label in both the row and the copy action', () => {
    render(
      <ul>
        <MetaSlugRow label="ID" value="abc123" />
      </ul>
    )
    expect(screen.getByText('ID')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /copy id/i })).toBeInTheDocument()
  })

  it('copies the value to the clipboard when the chip is clicked', async () => {
    const writeText = jest.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    render(
      <ul>
        <MetaSlugRow value="copy-me" />
      </ul>
    )
    fireEvent.click(screen.getByRole('button', { name: /copy slug/i }))

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('copy-me')
    })
  })

  it('shows transient "Copied!" feedback that reverts after ~1.5s', async () => {
    jest.useFakeTimers()
    const writeText = jest.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    render(
      <ul>
        <MetaSlugRow value="feedback" />
      </ul>
    )
    const button = screen.getByRole('button', { name: /copy slug/i })

    fireEvent.click(button)
    // Flush the writeText promise so the copied state turns on.
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(button).toHaveAttribute('title', 'Copied!')

    act(() => {
      jest.advanceTimersByTime(1500)
    })
    expect(button).toHaveAttribute('title', 'Copy slug')

    jest.useRealTimers()
  })

  it('renders the chip as a real, keyboard-focusable <button>', () => {
    render(
      <ul>
        <MetaSlugRow value="semantic" />
      </ul>
    )
    const button = screen.getByRole('button', { name: /copy slug/i })
    expect(button.tagName).toBe('BUTTON')
    expect(button).toHaveAttribute('type', 'button')
  })

  // Placed last: userEvent.setup() installs its own clipboard stub on
  // `navigator`, so this test reads back through that stub. Delete any stub left
  // by earlier tests first so setup() installs a fresh, readable one.
  it('copies the value when the chip is activated via the keyboard', async () => {
    Reflect.deleteProperty(navigator, 'clipboard')
    const user = userEvent.setup()

    render(
      <ul>
        <MetaSlugRow value="kbd-copy" />
      </ul>
    )
    screen.getByRole('button', { name: /copy slug/i }).focus()
    await user.keyboard('{Enter}')

    await waitFor(async () => {
      expect(await navigator.clipboard.readText()).toBe('kbd-copy')
    })
  })
})

describe('MetadataPanel', () => {
  it('renders the default "Metadata" heading inside the panel card', () => {
    render(<MetadataPanel createdAt={CREATED} updatedAt={CREATED} />)
    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.getByTestId('metadata-panel')).toBeInTheDocument()
  })

  it('accepts a custom title', () => {
    render(<MetadataPanel title="Details" createdAt={CREATED} />)
    expect(screen.getByText('Details')).toBeInTheDocument()
  })

  it('shows a "Created" row when createdAt is provided', () => {
    render(<MetadataPanel createdAt={CREATED} updatedAt={CREATED} />)
    expect(screen.getByText('Created')).toBeInTheDocument()
  })

  it('hides the "Updated" row when updatedAt equals createdAt', () => {
    render(<MetadataPanel createdAt={CREATED} updatedAt={CREATED} />)
    expect(screen.queryByText('Updated')).not.toBeInTheDocument()
  })

  it('shows the "Updated" row when updatedAt differs from createdAt', () => {
    render(<MetadataPanel createdAt={CREATED} updatedAt={UPDATED} />)
    expect(screen.getByText('Updated')).toBeInTheDocument()
  })

  it('shows "Updated" even without a createdAt', () => {
    render(<MetadataPanel updatedAt={UPDATED} />)
    expect(screen.getByText('Updated')).toBeInTheDocument()
    expect(screen.queryByText('Created')).not.toBeInTheDocument()
  })

  it('renders no time rows when neither date is provided', () => {
    render(
      <MetadataPanel>
        <MetaRow label="Type">general</MetaRow>
      </MetadataPanel>
    )
    expect(screen.queryByText('Created')).not.toBeInTheDocument()
    expect(screen.queryByText('Updated')).not.toBeInTheDocument()
  })

  it('renders leading rows before the Created/Updated rows', () => {
    render(
      <MetadataPanel createdAt={CREATED} updatedAt={UPDATED}>
        <MetaRow label="Status">active</MetaRow>
      </MetadataPanel>
    )
    const labels = screen
      .getAllByText(/^(Status|Created|Updated)$/)
      .map(el => el.textContent)
    expect(labels).toEqual(['Status', 'Created', 'Updated'])
  })

  it('renders a compact relative label for a recent created date', () => {
    const FIXED_NOW = new Date('2024-06-01T12:00:00Z').getTime()
    jest.useFakeTimers()
    jest.setSystemTime(FIXED_NOW)
    const recent = new Date(FIXED_NOW - 5000).toISOString()
    render(<MetadataPanel createdAt={recent} updatedAt={recent} />)
    expect(screen.getByText('just now')).toBeInTheDocument()
    jest.useRealTimers()
  })
})

describe('MetadataPanel version history', () => {
  function renderWithRouter(ui: ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>)
  }

  it('renders no version-history footer when the prop is omitted', () => {
    render(<MetadataPanel createdAt={CREATED} />)
    expect(
      screen.queryByTestId('metadata-version-history-link')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Version')).not.toBeInTheDocument()
  })

  it('renders the footer link with the count chip and target when provided', () => {
    renderWithRouter(
      <MetadataPanel
        createdAt={CREATED}
        versionHistory={{ count: 3, to: '/some/versions' }}
      />
    )
    const link = screen.getByTestId('metadata-version-history-link')
    expect(link).toHaveTextContent('View version history')
    expect(link).toHaveTextContent('3')
    expect(link).toHaveAttribute('href', '/some/versions')
  })

  it('honours a custom footer label', () => {
    renderWithRouter(
      <MetadataPanel
        versionHistory={{ count: 0, to: '/v', label: 'See history' }}
      />
    )
    expect(screen.getByText('See history')).toBeInTheDocument()
  })

  it('renders a "Version" row when currentVersion is supplied', () => {
    renderWithRouter(
      <MetadataPanel
        versionHistory={{ count: 2, to: '/v', currentVersion: 3 }}
      />
    )
    expect(screen.getByText('Version')).toBeInTheDocument()
    expect(screen.getByText('v3')).toBeInTheDocument()
  })

  it('omits the "Version" row when currentVersion is not supplied', () => {
    renderWithRouter(<MetadataPanel versionHistory={{ count: 2, to: '/v' }} />)
    expect(screen.queryByText('Version')).not.toBeInTheDocument()
    // …but the footer link still renders.
    expect(
      screen.getByTestId('metadata-version-history-link')
    ).toBeInTheDocument()
  })

  it('suppresses the standalone "Updated" row when the version row carries editedAt', () => {
    renderWithRouter(
      <MetadataPanel
        createdAt={CREATED}
        updatedAt={UPDATED}
        versionHistory={{
          count: 1,
          to: '/v',
          currentVersion: 2,
          editedAt: UPDATED,
        }}
      />
    )
    expect(screen.queryByText('Updated')).not.toBeInTheDocument()
    expect(screen.getByText('Version')).toBeInTheDocument()
  })

  it('keeps the "Updated" row when version history omits editedAt', () => {
    renderWithRouter(
      <MetadataPanel
        createdAt={CREATED}
        updatedAt={UPDATED}
        versionHistory={{ count: 1, to: '/v', currentVersion: 2 }}
      />
    )
    expect(screen.getByText('Updated')).toBeInTheDocument()
  })
})
