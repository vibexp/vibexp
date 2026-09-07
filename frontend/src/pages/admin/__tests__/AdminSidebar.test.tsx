import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router'

vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="scroll-area">{children}</div>
  ),
}))

const mockSheetClose = vi.hoisted(() => vi.fn())

// Mirrors `MobileSidebar.test.tsx`: the mock replicates Radix Slot's className
// string-join, so a function `className` would show up as garbage classes.
vi.mock('@/components/ui/sheet', async () => {
  const ReactActual = await vi.importActual<typeof React>('react')
  return {
    SheetClose: ({
      children,
      asChild,
    }: {
      children: React.ReactElement<{ className?: unknown }>
      asChild?: boolean
    }) => {
      if (asChild) {
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

import { AdminMobileSidebar, AdminSidebar } from '../AdminSidebar'

function renderRail(path = '/admin') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AdminSidebar />
    </MemoryRouter>
  )
}

function renderDrawer(path = '/admin') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AdminMobileSidebar />
    </MemoryRouter>
  )
}

// `classList.contains` and not a substring check: the INACTIVE class list
// carries `hover:bg-sidebar-accent/50`, which contains the active class as a
// substring and makes a `not.toContain` assertion vacuous.
const isHighlighted = (el: Element) =>
  el.classList.contains('bg-sidebar-accent')

describe('AdminSidebar', () => {
  beforeEach(() => {
    mockSheetClose.mockClear()
  })

  // ---------------------------------------------------------------------------
  // #891 — same structural invariant as the product rail: the element Radix
  // slots into `TooltipTrigger asChild` is the popper's anchor, so it must be
  // the `<a>` and not a box-less `display: contents` wrapper.
  // ---------------------------------------------------------------------------
  describe('tooltip trigger anchors on the nav link itself (#891)', () => {
    it('puts the Radix trigger state on the <a>', () => {
      renderRail()
      const link = screen.getByRole('link', { name: /^users$/i })
      expect(link).toHaveAttribute('data-state', 'closed')
    })

    it('renders no `display: contents` wrapper in the rail', () => {
      const { container } = renderRail()
      expect(container.querySelector('.contents')).toBeNull()
    })

    it('renders no `display: contents` wrapper in the drawer', () => {
      const { container } = renderDrawer()
      expect(container.querySelector('.contents')).toBeNull()
    })

    it('describes the hovered link itself once the tooltip opens', async () => {
      const user = userEvent.setup()
      renderRail()

      const link = screen.getByRole('link', { name: /^teams$/i })
      await user.hover(link)

      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent('Teams')
      expect(link).toHaveAttribute('aria-describedby', tooltip.id)
    })
  })

  describe('active-route highlighting parity', () => {
    it('highlights Dashboard only on the exact /admin path', () => {
      renderRail('/admin')
      expect(
        isHighlighted(screen.getByRole('link', { name: /^dashboard$/i }))
      ).toBe(true)
    })

    it.each(['/admin/users', '/admin/'])(
      'does NOT highlight Dashboard on %s (end: true)',
      path => {
        renderRail(path)
        expect(
          isHighlighted(screen.getByRole('link', { name: /^dashboard$/i }))
        ).toBe(false)
      }
    )

    // `/admin/` is why this does not use `useMatch`: `matchPath` compiles an
    // `end: true` pattern with a trailing `\/*$` and WOULD match it, while
    // NavLink's own check is strict equality and does not — leaving the row
    // styled active with no `aria-current`.
    it.each(['/admin', '/admin/', '/admin/users', '/admin/users/user-42'])(
      'keeps the highlight and aria-current in agreement on %s',
      path => {
        const { container } = renderRail(path)
        for (const link of container.querySelectorAll('a[href^="/admin"]')) {
          expect(
            isHighlighted(link),
            `${link.getAttribute('href')} on ${path}`
          ).toBe(link.hasAttribute('aria-current'))
        }
      }
    )

    // Only Dashboard declares `end`, so the `?? false` is load-bearing:
    // without it a detail page would stop highlighting its section.
    it.each(['/admin/users', '/admin/users/user-42'])(
      'highlights Users on %s (descendant match)',
      path => {
        renderRail(path)
        expect(
          isHighlighted(screen.getByRole('link', { name: /^users$/i }))
        ).toBe(true)
      }
    )
  })

  describe('drawer rows survive SheetClose asChild (Radix Slot)', () => {
    it('keeps the Tailwind classes on a drawer row', () => {
      renderDrawer('/admin/projects')
      const link = screen.getByRole('link', { name: /^projects$/i })
      expect(link.className).toContain('flex')
      expect(link.className).toContain('px-2.5')
      expect(link.className).not.toContain('=>')
      expect(isHighlighted(link)).toBe(true)
    })

    it('closes the drawer when a row is clicked', async () => {
      const user = userEvent.setup()
      renderDrawer()

      await user.click(screen.getByRole('link', { name: /^teams$/i }))

      expect(mockSheetClose).toHaveBeenCalled()
    })
  })
})
