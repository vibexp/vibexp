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
    span: 'full',
    onClick: noop,
    testId: 'use-action',
  },
]

/** A form's rail: primary first, and both buttons share one row. */
const formActions: readonly ReadingAction[] = [
  {
    id: 'save',
    label: 'Save',
    icon: Wand2,
    emphasis: 'primary',
    onClick: noop,
    testId: 'save-action',
  },
  {
    id: 'cancel',
    label: 'Cancel',
    icon: ArrowLeft,
    onClick: noop,
    testId: 'cancel-action',
  },
]

describe('ReadingActions', () => {
  it('spans an action that asks for the full row across both grid columns (#917)', () => {
    render(<ReadingActions actions={actions} layout="grid" />)

    // A long label clips inside a half-width cell, which is what the gallery's
    // "Use this prompt" hit.
    expect(screen.getByTestId('use-action')).toHaveClass('col-span-2')
    expect(screen.getByTestId('back-action')).not.toHaveClass('col-span-2')
  })

  it('leaves a primary action that did not ask for it unspanned', () => {
    render(<ReadingActions actions={formActions} layout="grid" />)

    // The resource form's rail is [Save (primary), Cancel]: spanning Save on
    // emphasis alone would strand Cancel in a half-width cell on row 2.
    expect(screen.getByTestId('save-action')).not.toHaveClass('col-span-2')
    expect(screen.getByTestId('cancel-action')).not.toHaveClass('col-span-2')
  })

  it('leaves the chips layout unspanned', () => {
    render(<ReadingActions actions={actions} layout="chips" />)

    expect(screen.getByTestId('use-action')).not.toHaveClass('col-span-2')
  })
})
