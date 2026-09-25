import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'

import { TriStateFilter } from '../TriStateFilter'

describe('TriStateFilter', () => {
  it('marks the current value as checked', () => {
    render(
      <TriStateFilter label="Has projects" value={true} onChange={vi.fn()} />
    )
    expect(
      screen.getByRole('radiogroup', { name: 'Has projects' })
    ).toBeVisible()
    expect(screen.getByRole('radio', { name: 'Yes' })).toHaveAttribute(
      'aria-checked',
      'true'
    )
    expect(screen.getByRole('radio', { name: 'Any' })).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })

  it.each([
    ['Yes', true],
    ['No', false],
  ] as const)('selects %s', async (name, expected) => {
    const onChange = vi.fn()
    render(
      <TriStateFilter
        label="Has projects"
        value={undefined}
        onChange={onChange}
      />
    )
    await userEvent.click(screen.getByRole('radio', { name }))
    expect(onChange).toHaveBeenCalledWith(expected)
  })

  it('returns to any', async () => {
    const onChange = vi.fn()
    render(
      <TriStateFilter label="Has projects" value={false} onChange={onChange} />
    )
    await userEvent.click(screen.getByRole('radio', { name: 'Any' }))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('ignores a click on the current option', async () => {
    const onChange = vi.fn()
    render(
      <TriStateFilter label="Has projects" value={false} onChange={onChange} />
    )
    await userEvent.click(screen.getByRole('radio', { name: 'No' }))
    expect(onChange).not.toHaveBeenCalled()
  })
})
