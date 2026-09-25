import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { endOfDay, startOfDay } from 'date-fns'

import { DateTimeRangeFilter } from '../DateTimeRangeFilter'

// Friday 24 July 2026, mid-afternoon local time.
const NOW = new Date(2026, 6, 24, 15, 30, 0)

describe('DateTimeRangeFilter', () => {
  it('labels the group and shows the "Any time" placeholder', () => {
    render(
      <DateTimeRangeFilter
        label="Last resource created"
        value={{}}
        onChange={vi.fn()}
        now={NOW}
      />
    )
    expect(
      screen.getByRole('group', { name: 'Last resource created' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Last resource created' })
    ).toHaveTextContent('Any time')
  })

  it('passes a picked preset through', async () => {
    const onChange = vi.fn()
    render(
      <DateTimeRangeFilter
        label="Last resource created"
        value={{}}
        onChange={onChange}
        now={NOW}
      />
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Last resource created' })
    )
    await userEvent.click(screen.getByRole('button', { name: 'Last 7 days' }))
    expect(onChange).toHaveBeenCalledWith({
      from: startOfDay(new Date(2026, 6, 18)),
      to: endOfDay(NOW),
    })
  })
})
