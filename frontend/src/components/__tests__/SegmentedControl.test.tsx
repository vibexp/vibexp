import { fireEvent, render, screen } from '@testing-library/react'

import { SegmentedControl } from '../SegmentedControl'

const OPTIONS = [
  { value: '7d', label: '7 days' },
  { value: '30d', label: '30 days' },
  { value: '90d', label: '3 months' },
]

describe('SegmentedControl', () => {
  it('renders every option and marks the active one selected', () => {
    render(
      <SegmentedControl
        options={OPTIONS}
        value="30d"
        onChange={() => undefined}
        aria-label="range"
      />
    )

    expect(screen.getAllByRole('tab')).toHaveLength(3)
    expect(screen.getByRole('tab', { name: '30 days' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('tab', { name: '7 days' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
  })

  it('calls onChange with the option value when a segment is clicked', () => {
    const onChange = vi.fn()
    render(
      <SegmentedControl options={OPTIONS} value="30d" onChange={onChange} />
    )

    fireEvent.click(screen.getByRole('tab', { name: '3 months' }))
    expect(onChange).toHaveBeenCalledWith('90d')
  })

  it('shows a disabled option but never selects it', () => {
    const onChange = vi.fn()
    render(
      <SegmentedControl
        options={[
          ...OPTIONS,
          { value: 'all', label: 'All time', disabled: true },
        ]}
        value="30d"
        onChange={onChange}
      />
    )

    const option = screen.getByRole('tab', { name: 'All time' })
    expect(option).toBeDisabled()
    expect(option).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(option)
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('tab', { name: '7 days' })).toBeEnabled()
  })
})
