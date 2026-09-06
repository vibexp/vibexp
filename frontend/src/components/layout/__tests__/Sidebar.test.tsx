import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router'

// ScrollArea needs browser APIs jsdom lacks, and it is not what these tests are
// about — the rail markup inside it is.
vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="scroll-area">{children}</div>
  ),
}))

const mockUseShell = vi.hoisted(() => vi.fn())
vi.mock('@/components/layout/ShellContext', () => ({
  useShell: () => mockUseShell() as unknown,
}))

import { Sidebar } from '../Sidebar'

/** `expanded: false` is the collapsed icon rail — the form the tooltips serve. */
function renderRail(path = '/', expanded = false) {
  mockUseShell.mockReturnValue({ navExpanded: expanded })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Sidebar />
    </MemoryRouter>
  )
}

describe('Sidebar', () => {
  beforeEach(() => {
    mockUseShell.mockReset()
  })

  // ---------------------------------------------------------------------------
  // #891: the tooltip trigger must BE the nav link.
  //
  // jsdom has no layout engine, so it cannot see the symptom (a bubble at the
  // viewport origin). What it can see is the cause: Radix renders
  // `TooltipTrigger asChild` as `PopperPrimitive.Anchor asChild`, so whatever
  // element receives the trigger props is the box floating-ui measures. A
  // `display: contents` wrapper generates no principal box → 0x0 at (0,0).
  // These assertions pin the structural invariant instead.
  // ---------------------------------------------------------------------------
  describe('tooltip trigger anchors on the nav link itself (#891)', () => {
    it('puts the Radix trigger state on the <a>, not on a wrapper', () => {
      renderRail()
      const link = screen.getByRole('link', { name: /^dashboard$/i })
      expect(link).toHaveAttribute('data-state', 'closed')
    })

    it('puts the trigger state on the collapsed group link too', () => {
      const { container } = renderRail()
      const groupLink = container.querySelector('a[href="/prompts"]')
      expect(groupLink).not.toBeNull()
      expect(groupLink).toHaveAttribute('data-state', 'closed')
    })

    it('renders no box-less `display: contents` wrapper anywhere', () => {
      const { container } = renderRail()
      expect(container.querySelector('.contents')).toBeNull()
    })

    it('describes the hovered link itself once the tooltip opens', async () => {
      const user = userEvent.setup()
      renderRail()

      const link = screen.getByRole('link', { name: /^blueprints$/i })
      await user.hover(link)

      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent('Blueprints')
      // The described element is the anchor. If a wrapper were slotted, this
      // attribute would land on the wrapper and the link would carry nothing.
      expect(link).toHaveAttribute('aria-describedby', tooltip.id)
      expect(link).toHaveAttribute('data-state', 'delayed-open')
    })
  })

  // ---------------------------------------------------------------------------
  // Active state moved from NavLink's function `className` to `useMatch`, so
  // the highlight has to be shown to be unchanged — including the `end`
  // semantics that make `/` exact-match only.
  // ---------------------------------------------------------------------------
  describe('active-route highlighting parity', () => {
    // `classList.contains` and not a substring check: the INACTIVE class list
    // carries `hover:bg-sidebar-accent/50`, which contains the active class as
    // a substring and makes a `not.toContain` assertion vacuous.
    const isHighlighted = (el: Element | null | undefined) =>
      el?.classList.contains('bg-sidebar-accent') ?? false

    it('highlights Dashboard on / and marks it the current page', () => {
      renderRail('/')
      const link = screen.getByRole('link', { name: /^dashboard$/i })
      expect(isHighlighted(link)).toBe(true)
      expect(link).toHaveAttribute('aria-current', 'page')
    })

    it.each(['/prompts', '/blueprints'])(
      'does NOT highlight Dashboard on %s (end-match on /)',
      path => {
        renderRail(path)
        const link = screen.getByRole('link', { name: /^dashboard$/i })
        expect(isHighlighted(link)).toBe(false)
        expect(link).not.toHaveAttribute('aria-current')
      }
    )

    it.each([
      '/blueprints',
      '/blueprints/3f1c2a90-0000-4000-8000-000000000001',
    ])('highlights Blueprints on %s (descendant match)', path => {
      renderRail(path)
      const link = screen.getByRole('link', { name: /^blueprints$/i })
      expect(isHighlighted(link)).toBe(true)
    })

    it.each(['/prompts', '/prompts/some-prompt-slug'])(
      'highlights the collapsed Prompts group link on %s',
      path => {
        const { container } = renderRail(path)
        const groupLink = container.querySelector('a[href="/prompts"]')
        expect(isHighlighted(groupLink)).toBe(true)
      }
    )

    it('never stringifies a function className onto a rail link', () => {
      const { container } = renderRail()
      for (const link of container.querySelectorAll('a')) {
        expect(link.className).not.toContain('=>')
      }
    })
  })
})
