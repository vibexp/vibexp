/**
 * Calendar primitive — the nav chevrons (#1033).
 *
 * The `Chevron` override lives at module scope, so `react-day-picker` sees one
 * stable component type across renders. The identity assertion below is what
 * catches a regression back to an inline arrow in the `components` prop: a new
 * type per render makes React unmount and remount the chevron subtree, handing
 * back fresh DOM nodes rather than the ones it rendered before.
 */
import { render, screen } from '@testing-library/react'

import { Calendar } from '@/components/ui/calendar'

// Friday 24 July 2026 — fixed, so the rendered month never depends on the day
// CI happens to run on.
const MONTH = new Date(2026, 6, 24)

it('renders one left and one right nav chevron, sized like every other icon', () => {
  render(<Calendar defaultMonth={MONTH} />)

  expect(screen.getByTestId('chevronleft-icon')).toHaveClass('size-4')
  expect(screen.getByTestId('chevronright-icon')).toHaveClass('size-4')
})

it('keeps the same chevron elements when Calendar re-renders', () => {
  const { rerender } = render(<Calendar defaultMonth={MONTH} />)
  const previous = screen.getByTestId('chevronleft-icon')
  const next = screen.getByTestId('chevronright-icon')

  rerender(<Calendar defaultMonth={MONTH} className="mt-2" />)

  expect(screen.getByTestId('chevronleft-icon')).toBe(previous)
  expect(screen.getByTestId('chevronright-icon')).toBe(next)
})
