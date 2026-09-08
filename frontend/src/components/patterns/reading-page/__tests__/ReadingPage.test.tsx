import { render, screen, waitFor, within } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { ArrowLeft, Info, MessageSquare, Pencil, Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'

import { ShellProvider, useShell } from '@/components/layout/ShellContext'
import { Panel, usePanelPresentation } from '@/components/ui/panel'
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

  // The details column is pinned to the right edge of the content area, so the
  // article centers itself in whatever is left beside it — unconditionally, in
  // every rail state (#890). Making it conditional (#888/#889) is what pushed
  // the article against the row's left edge next to a floating column.
  describe('centering contract', () => {
    function article() {
      const el = screen.getByTestId('reading-page').querySelector('article')
      expect(el).not.toBeNull()
      return el!
    }

    it('centers itself beside the open details column', () => {
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-state',
        'open'
      )
      expect(article()).toHaveClass('mx-auto', 'max-w-[72ch]', 'w-full')
    })

    it('centers itself beside the collapsed rail', () => {
      storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-state',
        'collapsed'
      )
      expect(article()).toHaveClass('mx-auto', 'max-w-[72ch]', 'w-full')
    })

    it('centers itself when there is no details rail at all', () => {
      renderPage(<ReadingPage title="Doc">body</ReadingPage>)
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
      expect(article()).toHaveClass('mx-auto', 'max-w-[72ch]', 'w-full')
    })

    it('centers itself below lg, where the details are a sheet', () => {
      viewport.setWidth(900)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
      expect(article()).toHaveClass('mx-auto', 'max-w-[72ch]', 'w-full')
    })

    // AC of #916: a detail page and its edit page must share the measure and
    // the gutters, so leaving edit mode is not a visual jump. Compared as one
    // string rather than by spot-checking classes: any divergence at all —
    // padding, width cap, auto margins — fails here.
    it('gives the editing presentation the identical article box', () => {
      const reading = renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const readingClasses = article().className
      reading.unmount()

      renderPage(
        <ReadingPage
          title="Doc"
          presentation="editing"
          actions={ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      expect(article().className).toBe(readingClasses)
      expect(article()).toHaveClass('mx-auto', 'max-w-[72ch]', 'w-full')
    })

    // The column and the rail are the same <aside>, so the flush-right
    // guarantee is that nothing sits between it and the end of the row.
    it('puts the details column last in the row, with nothing after it', () => {
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const aside = screen.getByTestId('reading-details')
      expect(aside.nextElementSibling).toBeNull()
      expect(aside).toHaveClass('shrink-0')
    })
  })

  // The column is itself a bordered, padded surface, so its widgets render
  // flat: no second border, shadow, radius or background inside it (#890).
  describe('flat panel presentation', () => {
    function PresentationProbe() {
      return <span data-testid="presentation">{usePanelPresentation()}</span>
    }

    const PROBE_SECTIONS: ReadingSection[] = [
      {
        id: 'metadata',
        label: 'Metadata',
        icon: Info,
        content: (
          <Panel data-testid="probe-panel">
            <PresentationProbe />
          </Panel>
        ),
      },
    ]

    it('renders its sections flat in the desktop column', () => {
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={PROBE_SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('presentation')).toHaveTextContent('flat')
      const panel = screen.getByTestId('probe-panel')
      expect(panel).not.toHaveClass('rounded-lg', 'border', 'shadow-sm')
      expect(panel.className).toBe('')
    })

    it('renders its sections flat inside the sheet too', async () => {
      const user = userEvent.setup()
      viewport.setWidth(900)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={PROBE_SECTIONS}>
          body
        </ReadingPage>
      )
      await user.click(screen.getByRole('button', { name: 'open sheet' }))
      expect(await screen.findByTestId('presentation')).toHaveTextContent(
        'flat'
      )
    })

    // Card is the default everywhere else, so a widget dropped on a dashboard
    // is unaffected by any of this.
    it('leaves panels outside the column as cards', () => {
      render(<Panel data-testid="loose-panel" />)
      expect(screen.getByTestId('loose-panel')).toHaveClass(
        'rounded-lg',
        'border',
        'shadow-sm'
      )
    })
  })

  // The design's document actions are 13px with 7px/10px padding, not the
  // generic `sm` button (#890).
  it('sizes the action grid to the design scale', () => {
    renderPage(
      <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
        body
      </ReadingPage>
    )
    const edit = within(screen.getByTestId('details-column')).getByTestId(
      'edit-button'
    )
    expect(edit).toHaveClass('text-[13px]', 'px-2.5', 'py-[7px]')
    expect(edit).not.toHaveClass('h-9')
  })

  // Editing is the same shell hosting a form (#916): everything structural is
  // shared, and `presentation` only marks the mode and keeps the details panel
  // registered while an edit page is still loading its resource.
  describe('editing presentation', () => {
    const EDIT_ACTIONS: ReadingAction[] = [
      {
        id: 'save',
        label: 'Save changes',
        icon: Pencil,
        emphasis: 'primary',
        onClick: vi.fn(),
        testId: 'save-button',
      },
      {
        id: 'cancel',
        label: 'Cancel',
        icon: ArrowLeft,
        onClick: vi.fn(),
        testId: 'cancel-button',
      },
    ]

    it('defaults to the reading presentation', () => {
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('reading-page')).toHaveAttribute(
        'data-presentation',
        'reading'
      )
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-presentation',
        'reading'
      )
    })

    it('marks the page and the details column as editing', () => {
      renderPage(
        <ReadingPage
          title="Edit doc"
          presentation="editing"
          actions={EDIT_ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('reading-page')).toHaveAttribute(
        'data-presentation',
        'editing'
      )
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-presentation',
        'editing'
      )
    })

    // An edit page paints before its resource resolves. Reading mode drops the
    // panel when there is nothing in it, which would make the header toggle and
    // the whole column appear only once the fetch lands.
    it('keeps the details panel registered with nothing to show yet', () => {
      renderPage(
        <ReadingPage title="Loading…" presentation="editing">
          <p>spinner</p>
        </ReadingPage>
      )
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details-registered',
        'true'
      )
      expect(screen.getByTestId('reading-details')).toBeInTheDocument()
    })

    it('drops the panel in reading mode with nothing to show', () => {
      renderPage(
        <ReadingPage title="Doc">
          <p>body</p>
        </ReadingPage>
      )
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details-registered',
        'false'
      )
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
    })

    it('renders Save as the solid primary action in the grid, Cancel outlined', () => {
      renderPage(
        <ReadingPage
          title="Edit doc"
          presentation="editing"
          actions={EDIT_ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      const column = within(screen.getByTestId('details-column'))
      expect(column.getByTestId('save-button')).toHaveClass('bg-primary')
      expect(column.getByTestId('cancel-button')).not.toHaveClass('bg-primary')
    })

    // 768–1023px has no rail (that starts at lg) and, in reading mode, no
    // chips either — the actions live in a sheet that opens closed. Fine for
    // Copy and Delete, not for a form's primary action.
    it('keeps Save reachable as a chip between md and lg', () => {
      viewport.setWidth(900)
      renderPage(
        <ReadingPage
          title="Edit doc"
          presentation="editing"
          actions={EDIT_ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      expect(screen.queryByTestId('reading-details')).not.toBeInTheDocument()
      const chips = within(screen.getByTestId('reading-actions-chips'))
      expect(chips.getByTestId('save-button')).toBeInTheDocument()
      expect(chips.getByTestId('cancel-button')).toBeInTheDocument()
    })

    it('leaves the reading presentation without chips between md and lg', () => {
      viewport.setWidth(900)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      expect(
        screen.queryByTestId('reading-actions-chips')
      ).not.toBeInTheDocument()
    })

    it('renders Save and Cancel as chips under the title on phones', () => {
      viewport.setWidth(600)
      renderPage(
        <ReadingPage
          title="Edit doc"
          presentation="editing"
          actions={EDIT_ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      const chips = within(screen.getByTestId('reading-actions-chips'))
      expect(chips.getByTestId('save-button')).toBeInTheDocument()
      expect(chips.getByTestId('cancel-button')).toBeInTheDocument()
    })

    it('folds to the icon rail like the detail page does', () => {
      storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
      renderPage(
        <ReadingPage
          title="Edit doc"
          presentation="editing"
          actions={EDIT_ACTIONS}
          sections={SECTIONS}
        >
          body
        </ReadingPage>
      )
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-state',
        'collapsed'
      )
      expect(
        within(screen.getByTestId('details-rail')).getByTestId('save-button')
      ).toBeInTheDocument()
    })
  })

  // Delete is the fourth outlined action in the grid, not the one solid red
  // button — and an outlined destructive on the rail (#890).
  describe('destructive action styling', () => {
    it('renders Delete outlined in the action grid', () => {
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const del = within(screen.getByTestId('details-column')).getByTestId(
        'delete-button'
      )
      expect(del).toHaveClass('border-destructive', 'text-destructive')
      expect(del).not.toHaveClass('bg-destructive')
    })

    it('renders Delete in the destructive colour on the folded rail', () => {
      storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const del = within(screen.getByTestId('details-rail')).getByTestId(
        'delete-button'
      )
      expect(del).toHaveClass('text-destructive')
      expect(del).not.toHaveClass('bg-destructive')
    })

    it('renders Delete outlined as a phone chip', () => {
      viewport.setWidth(600)
      renderPage(
        <ReadingPage title="Doc" actions={ACTIONS} sections={SECTIONS}>
          body
        </ReadingPage>
      )
      const del = within(
        screen.getByTestId('reading-actions-chips')
      ).getByTestId('delete-button')
      expect(del).toHaveClass('border-destructive', 'text-destructive')
      expect(del).not.toHaveClass('bg-destructive')
    })
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
