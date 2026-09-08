import { render, screen } from '@testing-library/react'
import { ArrowLeft, Wand2 } from 'lucide-react'

import { ReadingActions } from '../ReadingActions'
import type { ReadingAction } from '../types'

const noop = () => undefined

const actions: readonly ReadingAction[] = [
  {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: noop,
    testId: 'back-action',
  },
  {
    id: 'use',
    label: 'Use this prompt',
    icon: Wand2,
    emphasis: 'primary',
    onClick: noop,
    testId: 'use-action',
  },
]

describe('ReadingActions', () => {
  it('spans the primary action across both columns of the grid (#917)', () => {
    render(<ReadingActions actions={actions} layout="grid" />)

    // A long primary label clips inside a half-width cell, which is what the
    // gallery's "Use this prompt" hit.
    expect(screen.getByTestId('use-action')).toHaveClass('col-span-2')
    expect(screen.getByTestId('back-action')).not.toHaveClass('col-span-2')
  })

  it('leaves the chips layout unspanned', () => {
    render(<ReadingActions actions={actions} layout="chips" />)

    expect(screen.getByTestId('use-action')).not.toHaveClass('col-span-2')
  })
})
