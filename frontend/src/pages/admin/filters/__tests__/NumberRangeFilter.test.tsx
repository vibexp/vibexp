import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'

import { NumberRangeFilter } from '../NumberRangeFilter'

function renderFilter(value = {}) {
  const onChange = vi.fn()
  const view = render(
    <NumberRangeFilter label="Prompts" value={value} onChange={onChange} />
  )
  return {
    onChange,
    min: screen.getByLabelText('Prompts minimum'),
    max: screen.getByLabelText('Prompts maximum'),
    ...view,
  }
}

describe('NumberRangeFilter', () => {
  it('commits on blur, not per keystroke', async () => {
    const { onChange, min } = renderFilter()
    await userEvent.type(min, '10')
    expect(onChange).not.toHaveBeenCalled()
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith({ min: 10, max: undefined })
  })

  it('commits on Enter', async () => {
    const { onChange, max } = renderFilter()
    await userEvent.type(max, '4{Enter}')
    expect(onChange).toHaveBeenCalledWith({ min: undefined, max: 4 })
  })

  it('clears a bound when emptied', async () => {
    const { onChange, min } = renderFilter({ min: 3 })
    expect(min).toHaveValue(3)
    await userEvent.clear(min)
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledWith({ min: undefined, max: undefined })
  })

  it('does not commit an unchanged value', async () => {
    const { onChange, min } = renderFilter({ min: 3 })
    await userEvent.click(min)
    await userEvent.tab()
    expect(onChange).not.toHaveBeenCalled()
  })

  it.each(['-1', '1.5'])('rejects %s inline without committing', async raw => {
    const { onChange, min } = renderFilter()
    await userEvent.type(min, `${raw}{Enter}`)
    expect(onChange).not.toHaveBeenCalled()
    expect(min).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('alert')).toHaveTextContent('Whole number ≥ 0')
  })

  it('rejects min above max', async () => {
    const { onChange, min, max } = renderFilter({ max: 2 })
    await userEvent.type(min, '5{Enter}')
    expect(onChange).not.toHaveBeenCalled()
    expect(min).toHaveAttribute('aria-invalid', 'true')
    expect(max).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('alert')).toHaveTextContent('Min must be ≤ max')
  })

  it('follows a value changed from outside and drops a stale error', async () => {
    const { min, rerender } = renderFilter({ min: 3 })
    await userEvent.clear(min)
    await userEvent.type(min, '-2{Enter}')
    expect(screen.getByRole('alert')).toBeInTheDocument()

    rerender(
      <NumberRangeFilter label="Prompts" value={{}} onChange={vi.fn()} />
    )
    expect(min).toHaveValue(null)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
