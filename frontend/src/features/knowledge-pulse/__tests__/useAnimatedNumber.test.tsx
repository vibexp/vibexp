import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AnimatedNumber } from '../AnimatedNumber'

describe('AnimatedNumber', () => {
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'performance'],
    })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('counts toward a new value instead of jumping, then lands on it', () => {
    const { rerender } = render(
      <AnimatedNumber value={500} durationMs={1000} />
    )
    expect(screen.getByText('500')).toBeInTheDocument()

    rerender(<AnimatedNumber value={700} durationMs={1000} />)
    act(() => {
      vi.advanceTimersByTime(500)
    })
    const mid = Number(screen.getByText(/\d/).textContent.replace(/,/g, ''))
    expect(mid).toBeGreaterThan(500)
    expect(mid).toBeLessThan(700)

    act(() => {
      vi.advanceTimersByTime(600)
    })
    expect(screen.getByText('700')).toBeInTheDocument()
  })

  it('jumps straight to the value when motion is disabled', () => {
    const { rerender } = render(
      <AnimatedNumber value={1} durationMs={1000} disabled />
    )
    rerender(<AnimatedNumber value={1234} durationMs={1000} disabled />)
    expect(screen.getByText('1,234')).toBeInTheDocument()
  })
})
