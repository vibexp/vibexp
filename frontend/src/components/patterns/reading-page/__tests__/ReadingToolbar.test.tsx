import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Copy, History, Lock, Pencil, Trash2 } from 'lucide-react'

import { ReadingToolbar, splitToolbarActions } from '../ReadingToolbar'
import { openMoreActions } from '../testing'
import type { ReadingAction } from '../types'

const action = (
  id: string,
  overrides: Partial<ReadingAction> = {}
): ReadingAction => ({
  id,
  label: id,
  icon: Pencil,
  onClick: vi.fn(),
  testId: `${id}-action`,
  ...overrides,
})

const EDIT = action('Edit', { placement: 'toolbar', emphasis: 'primary' })
const COPY = action('Copy content', { placement: 'toolbar', icon: Copy })
const LOCK = action('Lock', { icon: Lock })
const HISTORY = action('Version history', { icon: History })
const DELETE = action('Delete', { tone: 'destructive', icon: Trash2 })

describe('splitToolbarActions', () => {
  it('puts toolbar actions on the bar, the rest in the menu, destructive last', () => {
    const split = splitToolbarActions(
      [DELETE, EDIT, LOCK, COPY, HISTORY],
      false
    )

    expect(split.buttons).toEqual([EDIT, COPY])
    expect(split.menu).toEqual([LOCK, HISTORY])
    expect(split.destructive).toEqual([DELETE])
  })

  it('keeps one button on a phone and moves the others to the top of the menu', () => {
    const split = splitToolbarActions([EDIT, COPY, LOCK, DELETE], true)

    expect(split.buttons).toEqual([EDIT])
    expect(split.menu).toEqual([COPY, LOCK])
    expect(split.destructive).toEqual([DELETE])
  })

  it('offers only what it is handed: an action the page left out is nowhere', () => {
    // Permission gating is the page's: it leaves out what the server's
    // `permissions` array does not grant, and the toolbar adds nothing back.
    const split = splitToolbarActions([EDIT, COPY], false)

    expect(split.menu).toEqual([])
    expect(split.destructive).toEqual([])
  })
})

describe('ReadingToolbar', () => {
  it('shows Edit as the primary labelled button and Copy beside it', () => {
    render(<ReadingToolbar actions={[EDIT, COPY, LOCK, DELETE]} />)

    const toolbar = screen.getByRole('toolbar', { name: 'Document actions' })
    expect(within(toolbar).getByRole('button', { name: 'Edit' })).toHaveClass(
      'bg-primary'
    )
    expect(
      within(toolbar).getByRole('button', { name: 'Copy content' })
    ).toBeInTheDocument()
    // Delete is not a button on the bar.
    expect(
      within(toolbar).queryByRole('button', { name: 'Delete' })
    ).not.toBeInTheDocument()
  })

  it('lists the other actions in the More menu, with Delete last after a separator', async () => {
    const user = userEvent.setup()
    render(<ReadingToolbar actions={[DELETE, EDIT, LOCK, HISTORY]} />)

    await openMoreActions()
    const menu = screen.getByRole('menu')
    expect(
      within(menu)
        .getAllByRole('menuitem')
        .map(item => item.textContent)
    ).toEqual(['Lock', 'Version history', 'Delete'])
    expect(within(menu).getByRole('separator')).toBeInTheDocument()

    await user.click(within(menu).getByRole('menuitem', { name: 'Delete' }))
    expect(DELETE.onClick).toHaveBeenCalledTimes(1)
  })

  it('has no More menu when no action needs one', () => {
    // A member without lock or delete rights is handed neither action, so
    // there is no empty menu to open.
    render(<ReadingToolbar actions={[EDIT, COPY]} />)

    expect(
      screen.queryByRole('button', { name: 'More actions' })
    ).not.toBeInTheDocument()
  })

  it('draws the Rendered / Raw switch for the body it is given', async () => {
    const user = userEvent.setup()
    const setMode = vi.fn()
    render(
      <ReadingToolbar
        actions={[EDIT]}
        bodyView={{ mode: 'rendered', setMode }}
      />
    )

    expect(screen.getByRole('tab', { name: 'Rendered' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(setMode).toHaveBeenCalledWith('raw')
  })

  it('moves the view switch into the menu on a phone', async () => {
    const user = userEvent.setup()
    const setMode = vi.fn()
    render(
      <ReadingToolbar
        compact
        actions={[EDIT, COPY]}
        bodyView={{ mode: 'rendered', setMode }}
      />
    )

    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    await openMoreActions()
    await user.click(screen.getByRole('menuitem', { name: 'View raw source' }))
    expect(setMode).toHaveBeenCalledWith('raw')
  })

  it('renders nothing with nothing to offer', () => {
    const { container } = render(<ReadingToolbar actions={[]} />)

    expect(container).toBeEmptyDOMElement()
  })
})
