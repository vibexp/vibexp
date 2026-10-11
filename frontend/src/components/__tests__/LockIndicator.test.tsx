import { render, screen } from '@testing-library/react'

import type { ResourceLock } from '@/types/resourceLock'

import { LockIndicator } from '../LockIndicator'

const lock: ResourceLock = {
  locked_by: { user_id: 'user-2', display_name: 'Ada Lovelace' },
  comment: 'Used by the nightly release agent',
  locked_at: '2026-10-11T10:15:30Z',
}

describe('LockIndicator', () => {
  it('names the locker in its accessible name', () => {
    render(<LockIndicator lock={lock} />)
    expect(
      screen.getByRole('img', { name: 'Locked by Ada Lovelace' })
    ).toBeInTheDocument()
  })

  it('renders nothing for an unlocked resource', () => {
    // `lock: null` is how the API says "unlocked", so an unlocked row must
    // cost no markup at all.
    const { container } = render(<LockIndicator lock={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the field is absent', () => {
    const { container } = render(<LockIndicator />)
    expect(container).toBeEmptyDOMElement()
  })

  it('does not shrink beside a truncating name, and keeps a caller class', () => {
    render(<LockIndicator lock={lock} className="ml-1" />)
    expect(screen.getByTestId('lock-indicator')).toHaveClass('shrink-0', 'ml-1')
  })
})
