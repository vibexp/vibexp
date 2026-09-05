import { act, render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import type { ReactNode } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

import { ShellProvider, useReadingShell, useShell } from '../ShellContext'

/** Exposes the shell state as DOM so tests can assert it and drive it. */
function Probe() {
  const shell = useShell()
  return (
    <div
      data-testid="probe"
      data-mode={shell.contentMode}
      data-nav={shell.navExpanded ? 'expanded' : 'collapsed'}
      data-details-registered={String(shell.detailsRegistered)}
      data-details={shell.detailsOpen ? 'open' : 'collapsed'}
      data-sheet={shell.detailsSheetOpen ? 'open' : 'closed'}
      data-desktop={String(shell.isDesktop)}
      data-tablet={String(shell.isTablet)}
    >
      <button type="button" onClick={shell.toggleNav}>
        toggle nav
      </button>
      <button type="button" onClick={shell.toggleDetails}>
        toggle details
      </button>
      <button
        type="button"
        onClick={() => {
          shell.setDetailsSheetOpen(true)
        }}
      >
        open sheet
      </button>
    </div>
  )
}

function ReadingPageStub({ details = true }: Readonly<{ details?: boolean }>) {
  useReadingShell({ details })
  return <div>reading page</div>
}

function renderShell(children?: ReactNode) {
  return render(
    <ShellProvider>
      <Probe />
      {children}
    </ShellProvider>
  )
}

describe('ShellContext', () => {
  let viewport: ReturnType<typeof mockViewportWidth>

  beforeEach(() => {
    storage.clear()
    viewport = mockViewportWidth(1280)
  })

  afterEach(() => {
    viewport.restore()
  })

  it('starts with everything open and in contained mode', () => {
    renderShell()
    const probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-mode', 'contained')
    expect(probe).toHaveAttribute('data-nav', 'expanded')
    expect(probe).toHaveAttribute('data-details', 'open')
    expect(probe).toHaveAttribute('data-details-registered', 'false')
    expect(probe).toHaveAttribute('data-desktop', 'true')
  })

  it('remembers the folded navigation per browser', async () => {
    const user = userEvent.setup()
    renderShell()
    await user.click(screen.getByRole('button', { name: 'toggle nav' }))
    expect(screen.getByTestId('probe')).toHaveAttribute('data-nav', 'collapsed')
    expect(storage.get(STORAGE_KEYS.NAV_COLLAPSED)).toBe('true')
    // Details are untouched: the two states are independent.
    expect(screen.getByTestId('probe')).toHaveAttribute('data-details', 'open')
    expect(storage.get(STORAGE_KEYS.DETAILS_COLLAPSED)).toBeNull()
  })

  it('remembers the folded details column per browser', async () => {
    const user = userEvent.setup()
    renderShell()
    await user.click(screen.getByRole('button', { name: 'toggle details' }))
    expect(screen.getByTestId('probe')).toHaveAttribute(
      'data-details',
      'collapsed'
    )
    expect(storage.get(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe('true')
    expect(screen.getByTestId('probe')).toHaveAttribute('data-nav', 'expanded')
  })

  it('restores a previously folded state on mount', () => {
    storage.set(STORAGE_KEYS.NAV_COLLAPSED, true)
    storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
    renderShell()
    const probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-nav', 'collapsed')
    expect(probe).toHaveAttribute('data-details', 'collapsed')
  })

  it('switches to reading mode and registers details while a reading page is mounted', () => {
    const { rerender } = render(
      <ShellProvider>
        <Probe />
        <ReadingPageStub />
      </ShellProvider>
    )
    let probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-mode', 'reading')
    expect(probe).toHaveAttribute('data-details-registered', 'true')

    rerender(
      <ShellProvider>
        <Probe />
      </ShellProvider>
    )
    probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-mode', 'contained')
    expect(probe).toHaveAttribute('data-details-registered', 'false')
  })

  it('enters reading mode without a details toggle when the page has no details', () => {
    renderShell(<ReadingPageStub details={false} />)
    const probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-mode', 'reading')
    expect(probe).toHaveAttribute('data-details-registered', 'false')
  })

  it('closes the details sheet when the viewport grows into the column layout', async () => {
    const user = userEvent.setup()
    viewport.setWidth(390)
    renderShell(<ReadingPageStub />)
    expect(screen.getByTestId('probe')).toHaveAttribute('data-desktop', 'false')
    await user.click(screen.getByRole('button', { name: 'open sheet' }))
    expect(screen.getByTestId('probe')).toHaveAttribute('data-sheet', 'open')

    act(() => {
      viewport.setWidth(1280)
    })
    expect(screen.getByTestId('probe')).toHaveAttribute('data-sheet', 'closed')
  })

  it('falls back to desktop defaults outside a provider', () => {
    render(<Probe />)
    const probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-desktop', 'true')
    expect(probe).toHaveAttribute('data-nav', 'expanded')
    expect(probe).toHaveAttribute('data-details', 'open')
  })
})
