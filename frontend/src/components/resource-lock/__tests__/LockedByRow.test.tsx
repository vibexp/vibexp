import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { ResourceLock } from '@/types/resourceLock'

import { LockedByRow } from '../LockedByRow'

const lock: ResourceLock = {
  locked_by: { user_id: 'user-2', display_name: 'Ada Lovelace' },
  comment: 'Used by the nightly release agent',
  locked_at: '2026-10-11T10:15:30Z',
}

function renderRow() {
  return render(
    <ul>
      <LockedByRow lock={lock} />
    </ul>
  )
}

describe('LockedByRow', () => {
  it('names the locker in a "Locked by" row', () => {
    renderRow()
    expect(screen.getByText('Locked by')).toBeInTheDocument()
    expect(screen.getByTestId('locked-by')).toHaveTextContent('Ada Lovelace')
  })

  // jsdom has no layout engine, so the tooltip's geometry belongs to the e2e
  // (#1294). What can be pinned here is the structural invariant behind #891:
  // the trigger state lands on a real, focusable element with a box of its
  // own, not on a `display: contents` wrapper.
  it('shows the lock comment on hover, anchored on the real button', async () => {
    const user = userEvent.setup()
    renderRow()
    const trigger = screen.getByTestId('locked-by')

    expect(trigger.tagName).toBe('BUTTON')
    expect(trigger).toHaveAttribute('type', 'button')
    expect(trigger.className).not.toContain('contents')
    expect(trigger).toHaveAttribute('data-state', 'closed')

    await user.hover(trigger)

    await waitFor(() => {
      expect(trigger).toHaveAttribute('data-state', 'delayed-open')
    })
    const tooltip = screen.getByRole('tooltip')
    expect(tooltip).toHaveTextContent('Used by the nightly release agent')
    expect(trigger).toHaveAttribute('aria-describedby', tooltip.id)
  })

  it('shows the lock comment on keyboard focus', async () => {
    const user = userEvent.setup()
    renderRow()
    const trigger = screen.getByTestId('locked-by')

    await user.tab()

    expect(trigger).toHaveFocus()
    await waitFor(() => {
      expect(trigger).toHaveAttribute('data-state', 'instant-open')
    })
    expect(screen.getByRole('tooltip')).toHaveTextContent(
      'Used by the nightly release agent'
    )
  })
})
