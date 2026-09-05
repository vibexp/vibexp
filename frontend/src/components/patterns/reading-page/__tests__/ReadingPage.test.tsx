import { render, screen, waitFor, within } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { ArrowLeft, Info, MessageSquare, Pencil, Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'

import { ShellProvider, useShell } from '@/components/layout/ShellContext'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

import { ReadingPage } from '../ReadingPage'
import type { ReadingAction, ReadingSection } from '../types'

function Probe() {
  const shell = useShell()
  return (
    <div
      data-testid="probe"
      data-mode={shell.contentMode}
      data-details-registered={String(shell.detailsRegistered)}
      data-details={shell.detailsOpen ? 'open' : 'collapsed'}
      data-sheet={shell.detailsSheetOpen ? 'open' : 'closed'}
    >
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

function renderPage(ui: ReactNode) {
  return render(
    <ShellProvider>
      <Probe />
      {ui}
    </ShellProvider>
  )
}

const onBack = vi.fn()
const onEdit = vi.fn()
const onDelete = vi.fn()

const ACTIONS: ReadingAction[] = [
  { id: 'back', label: 'Back', icon: ArrowLeft, onClick: onBack },
  {
    id: 'edit',
    label: 'Edit',
    icon: Pencil,
    onClick: onEdit,
    testId: 'edit-button',
  },
  {
    id: 'delete',
    label: 'Delete',
    icon: Trash2,
    tone: 'destructive',
    onClick: onDelete,
    testId: 'delete-button',
  },
]

const SECTIONS: ReadingSection[] = [
  {
    id: 'metadata',
    label: 'Metadata',
    icon: Info,
    content: <div>metadata widget</div>,
  },
  {
    id: 'comments',
    label: 'Comments',
    icon: MessageSquare,
    content: <div>comments widget</div>,
  },
  { id: 'hidden', label: 'Hidden', icon: Info, content: null },
]

describe('ReadingPage', () => {
  let viewport: ReturnType<typeof mockViewportWidth>

  beforeEach(() => {
    storage.clear()
    vi.clearAllMocks()
    viewport = mockViewportWidth(1280)
  })

  afterEach(() => {
    viewport.restore()
  })

  it('renders the title, description and body in the article', () => {
    renderPage(
      <ReadingPage title="Doc title" description="A short lead">
        <p>body text</p>
      </ReadingPage>
    )
    expect(
      screen.getByRole('heading', { level: 1, name: 'Doc title' })
    ).toBeInTheDocument()
    expect(screen.getByText('A short lead')).toBeInTheDocument()
    expect(screen.getByText('body text')).toBeInTheDocument()
    // Reading mode, but nothing to show in a details panel.
    const probe = screen.getByTestId('probe')
    expect(probe).toHaveAttribute('data-mode', 'reading')
    expect(probe).toHaveAttribute('data-details-registered', 'false')
    expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
  })

  describe('desktop column', () => {
    it('renders actions and sections in the open details column', async () => {
      const user = userEvent.setup()
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details-registered',
        'true'
      )
      const aside = screen.getByTestId('reading-details')
      expect(aside).toHaveAttribute('data-state', 'open')
      const column = within(aside).getByTestId('details-column')

      // Actions keep their test ids and fire their handlers.
      await user.click(within(column).getByTestId('edit-button'))
      expect(onEdit).toHaveBeenCalledTimes(1)
      await user.click(within(column).getByRole('button', { name: 'Back' }))
      expect(onBack).toHaveBeenCalledTimes(1)

      // Sections are anchored for the rail and labelled; empty ones vanish.
      expect(
        column.querySelector('[data-section="metadata"]')
      ).toHaveTextContent('metadata widget')
      expect(
        column.querySelector('[data-section="comments"]')
      ).toHaveTextContent('comments widget')
      expect(column.querySelector('[data-section="hidden"]')).toBeNull()

      // Desktop never renders the phone chips.
      expect(
        screen.queryByTestId('reading-actions-chips')
      ).not.toBeInTheDocument()
    })

    it('folds to the icon rail and reopens at the requested section', async () => {
      storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
      const scrollIntoView = vi.fn()
      Element.prototype.scrollIntoView = scrollIntoView
      const user = userEvent.setup()
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const aside = screen.getByTestId('reading-details')
      expect(aside).toHaveAttribute('data-state', 'collapsed')
      const rail = within(aside).getByTestId('details-rail')
      expect(within(aside).queryByTestId('details-column')).toBeNull()

      // Icon-only actions carry the same test ids and still work.
      await user.click(within(rail).getByTestId('delete-button'))
      expect(onDelete).toHaveBeenCalledTimes(1)
      expect(
        within(rail).getByRole('button', { name: 'Metadata' })
      ).toBeInTheDocument()
      expect(within(rail).queryByRole('button', { name: 'Hidden' })).toBeNull()

      await user.click(within(rail).getByRole('button', { name: 'Comments' }))
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details',
        'open'
      )
      expect(storage.get(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe('false')
      expect(
        within(screen.getByTestId('reading-details')).getByTestId(
          'details-column'
        )
      ).toBeInTheDocument()
      await waitFor(() => {
        expect(scrollIntoView).toHaveBeenCalled()
      })
      const scrolled = scrollIntoView.mock.instances[0] as HTMLElement
      expect(scrolled.getAttribute('data-section')).toBe('comments')
    })
  })

  describe('tablet (768–1024px)', () => {
    it('shows the details as a right-side sheet with the action grid', async () => {
      viewport.setWidth(900)
      const user = userEvent.setup()
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('reading-actions-chips')
      ).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'open sheet' }))
      const sheet = await screen.findByTestId('reading-details-sheet')
      expect(within(sheet).getByText('Details')).toBeInTheDocument()
      expect(
        within(sheet).getByTestId('reading-actions-grid')
      ).toBeInTheDocument()
      expect(within(sheet).getByText('comments widget')).toBeInTheDocument()
    })
  })

  describe('phone (< 768px)', () => {
    it('renders actions as chips under the title and details in a bottom sheet without actions', async () => {
      viewport.setWidth(390)
      const user = userEvent.setup()
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const chips = screen.getByTestId('reading-actions-chips')
      await user.click(within(chips).getByTestId('edit-button'))
      expect(onEdit).toHaveBeenCalledTimes(1)
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'open sheet' }))
      const sheet = await screen.findByTestId('reading-details-sheet')
      expect(within(sheet).queryByTestId('reading-actions-grid')).toBeNull()
      expect(within(sheet).getByText('metadata widget')).toBeInTheDocument()
    })
  })
})
