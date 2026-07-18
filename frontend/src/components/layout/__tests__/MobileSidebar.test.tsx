import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// ---------------------------------------------------------------------------
// Mock Radix-based UI primitives that require browser APIs unavailable in
// JSDOM (ResizeObserver used by ScrollArea, portal context for Sheet).
// ---------------------------------------------------------------------------
jest.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="scroll-area">{children}</div>
  ),
}))

// Spy recording every click that would close the Sheet via SheetClose.
const mockSheetClose = jest.fn()

jest.mock('@/components/ui/sheet', () => {
  const ReactActual = jest.requireActual<typeof React>('react')
  return {
    SheetClose: ({
      children,
      asChild,
    }: {
      children: React.ReactElement<{ className?: unknown }>
      asChild?: boolean
    }) => {
      if (asChild) {
        // Mimic Radix Slot: clone the child, merging props. Crucially, Slot
        // joins className values into a string — a function className (as
        // react-router's NavLink uses) gets stringified into garbage classes.
        // Replicating that here guards against regressions where NavLink is
        // slotted directly instead of via a `display: contents` wrapper.
        const child = ReactActual.Children.only(children)
        return ReactActual.cloneElement(child, {
          'data-testid': 'sheet-close',
          className: [child.props.className].filter(Boolean).join(' '),
          onClickCapture: () => mockSheetClose(),
        } as Partial<typeof child.props>)
      }
      return <button onClick={() => mockSheetClose()}>{children}</button>
    },
  }
})

import { MobileSidebar } from '../MobileSidebar'

function renderWithRouter(initialEntries: string[] = ['/']) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <MobileSidebar />
    </MemoryRouter>
  )
}

describe('MobileSidebar', () => {
  beforeEach(() => {
    mockSheetClose.mockClear()
  })

  // -------------------------------------------------------------------------
  // Fix 1: brand logo
  // -------------------------------------------------------------------------
  describe('brand header', () => {
    it('renders the design-system logo brand link navigating to /', () => {
      renderWithRouter()
      const homeLink = screen.getByRole('link', { name: /VibeXP/i })
      expect(homeLink).toBeInTheDocument()
      expect(homeLink).toHaveAttribute('href', '/')
      // Brand now uses the released @vibexp/design-system logo asset.
      const logo = homeLink.querySelector('img[alt="VibeXP"]')
      expect(logo).toBeInTheDocument()
    })

    it('does NOT render a plain "V" text badge span', () => {
      renderWithRouter()
      // The old implementation had <span>V</span> as the sole content.
      const badgeSpans = screen
        .queryAllByText('V')
        .filter(el => el.tagName === 'SPAN' && el.textContent === 'V')
      expect(badgeSpans).toHaveLength(0)
    })
  })

  // -------------------------------------------------------------------------
  // Leaf items render as flat nav links
  // -------------------------------------------------------------------------
  describe('leaf nav items render as flat links', () => {
    it('renders Dashboard as a nav link', () => {
      renderWithRouter()
      expect(
        screen.getByRole('link', { name: /^dashboard$/i })
      ).toBeInTheDocument()
    })

    it('renders Artifacts as a nav link', () => {
      renderWithRouter()
      expect(
        screen.getByRole('link', { name: /^artifacts$/i })
      ).toBeInTheDocument()
    })

    it('renders Blueprints as a nav link', () => {
      renderWithRouter()
      expect(
        screen.getByRole('link', { name: /^blueprints$/i })
      ).toBeInTheDocument()
    })

    it('renders Memories as a nav link', () => {
      renderWithRouter()
      expect(
        screen.getByRole('link', { name: /^memories$/i })
      ).toBeInTheDocument()
    })

    it('renders Settings as a nav link', () => {
      renderWithRouter()
      expect(
        screen.getByRole('link', { name: /^settings$/i })
      ).toBeInTheDocument()
    })
  })

  // -------------------------------------------------------------------------
  // Leaf links keep their Tailwind classes despite SheetClose's Slot merge.
  // Radix Slot stringifies NavLink's function `className`, which used to wipe
  // every class (icon and label rendered on separate lines on mobile).
  // -------------------------------------------------------------------------
  describe('nav link styling survives SheetClose asChild (Radix Slot)', () => {
    it('keeps the single-row flex classes on leaf nav links', () => {
      renderWithRouter()
      const link = screen.getByRole('link', { name: /^dashboard$/i })
      expect(link.className).toContain('flex')
      expect(link.className).toContain('items-center')
      expect(link.className).not.toContain('=>')
    })

    it('keeps classes on submenu links', () => {
      renderWithRouter(['/ai-tools/claude-code/overview'])
      const link = screen.getByRole('link', { name: /claude code/i })
      expect(link.className).toContain('rounded-md')
      expect(link.className).not.toContain('=>')
    })
  })

  // -------------------------------------------------------------------------
  // Fix 2: grouped items collapsed by default
  // -------------------------------------------------------------------------
  describe('grouped items are collapsed by default (route = /)', () => {
    it('renders "AI Tools" group trigger button', () => {
      renderWithRouter(['/'])
      expect(screen.getByText('AI Tools')).toBeInTheDocument()
    })

    it('renders "Prompts" group trigger button', () => {
      renderWithRouter(['/'])
      expect(screen.getByText('Prompts')).toBeInTheDocument()
    })

    it.each(['Claude Code', 'Cursor IDE', 'My Prompts'])(
      '"%s" child is hidden (data-state=closed) when route is "/"',
      childLabel => {
        renderWithRouter(['/'])
        // Radix Collapsible mounts the content but sets data-state="closed".
        const el = screen.queryByText(childLabel)
        if (el) {
          const content = el.closest('[data-state]')
          expect(content).toHaveAttribute('data-state', 'closed')
        } else {
          // Not rendered at all is also correct collapsed behaviour.
          expect(el).toBeNull()
        }
      }
    )
  })

  // -------------------------------------------------------------------------
  // Group is defaultOpen when route matches a child
  // -------------------------------------------------------------------------
  describe('group is defaultOpen when current route matches a child', () => {
    it.each([
      ['AI Tools', '/ai-tools/claude-code/overview', 'Claude Code'],
      ['AI Tools', '/ai-tools/cursor-ide/overview', 'Cursor IDE'],
      ['Prompts', '/prompt-gallery', 'Prompt Gallery'],
    ])('opens "%s" group when route is %s', (_group, route, childLabel) => {
      renderWithRouter([route])
      const el = screen.getByText(childLabel)
      const content = el.closest('[data-state]')
      expect(content).toHaveAttribute('data-state', 'open')
    })
  })

  // -------------------------------------------------------------------------
  // Clicking the trigger toggles children
  // -------------------------------------------------------------------------
  describe('clicking a group trigger toggles children visibility', () => {
    it('expands "AI Tools" children when trigger is clicked (starting collapsed)', async () => {
      const user = userEvent.setup()
      renderWithRouter(['/'])

      // Before click: collapsed
      const claudeBefore = screen.queryByText('Claude Code')
      if (claudeBefore) {
        expect(claudeBefore.closest('[data-state]')).toHaveAttribute(
          'data-state',
          'closed'
        )
      }

      const trigger = screen.getByText('AI Tools').closest('button')
      expect(trigger).not.toBeNull()
      await user.click(trigger!)

      // After click: open
      const claudeAfter = screen.getByText('Claude Code')
      expect(claudeAfter.closest('[data-state]')).toHaveAttribute(
        'data-state',
        'open'
      )
    })

    it('collapses "AI Tools" children on second click (starting open)', async () => {
      const user = userEvent.setup()
      renderWithRouter(['/ai-tools/claude-code/overview'])

      // Starts open — Claude Code is visible
      expect(
        screen.getByText('Claude Code').closest('[data-state]')
      ).toHaveAttribute('data-state', 'open')

      const trigger = screen.getByText('AI Tools').closest('button')
      expect(trigger).not.toBeNull()
      await user.click(trigger!)

      // After collapse, Radix sets hidden="" on the content — the element may
      // still be in the DOM but hidden, or queryByText returns null.
      // Either way, the CollapsibleContent should have data-state="closed".
      const contentEl = document.querySelector('[data-state="closed"].ml-6')
      expect(contentEl).toBeInTheDocument()
      expect(contentEl).toHaveAttribute('data-state', 'closed')
    })
  })

  // -------------------------------------------------------------------------
  // Issue #1574: sheet auto-closes on destination links, stays open on toggles
  // -------------------------------------------------------------------------
  describe('sheet auto-close on navigation (issue #1574)', () => {
    it('closes the sheet when a top-level leaf link is clicked', async () => {
      const user = userEvent.setup()
      renderWithRouter(['/'])

      await user.click(screen.getByRole('link', { name: /^artifacts$/i }))

      expect(mockSheetClose).toHaveBeenCalled()
    })

    it('closes the sheet when a submenu link is clicked', async () => {
      const user = userEvent.setup()
      // Group "AI Tools" is defaultOpen for this route, so the child link
      // is visible and clickable.
      renderWithRouter(['/ai-tools/claude-code/overview'])

      await user.click(screen.getByRole('link', { name: /claude code/i }))

      expect(mockSheetClose).toHaveBeenCalled()
    })

    it('closes the sheet when the logo link is clicked', async () => {
      const user = userEvent.setup()
      renderWithRouter(['/settings'])

      await user.click(screen.getByRole('link', { name: /VibeXP/i }))

      expect(mockSheetClose).toHaveBeenCalled()
    })

    it('does NOT close the sheet when a group toggle is expanded', async () => {
      const user = userEvent.setup()
      renderWithRouter(['/'])

      const trigger = screen.getByText('AI Tools').closest('button')
      expect(trigger).not.toBeNull()
      // The group toggle must not be wrapped in SheetClose at all …
      expect(trigger!.closest('[data-testid="sheet-close"]')).toBeNull()
      await user.click(trigger!)

      // … and clicking it must not signal the sheet to close.
      expect(mockSheetClose).not.toHaveBeenCalled()
    })

    it('does NOT close the sheet when a group toggle is collapsed', async () => {
      const user = userEvent.setup()
      // Group starts open for this route; clicking collapses it.
      renderWithRouter(['/ai-tools/claude-code/overview'])

      const trigger = screen.getByText('AI Tools').closest('button')
      expect(trigger).not.toBeNull()
      await user.click(trigger!)

      expect(mockSheetClose).not.toHaveBeenCalled()
    })
  })
})
