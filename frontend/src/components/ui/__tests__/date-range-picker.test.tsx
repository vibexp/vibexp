/**
 * DateRangePicker (#457).
 *
 * The clock is injected via the `now` prop rather than faked globally, so the
 * expectations below are fixed calendar dates instead of arithmetic against
 * whatever day CI runs on.
 */
import { render, screen, within } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { useState } from 'react'

import type { DateRangeValue } from '@/components/ui/date-range'
import {
  DEFAULT_RANGE_PRESETS,
  formatRangeLabel,
  fromDateParam,
  rangeForPreset,
  rangeToInstants,
  toDateParam,
} from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'

// Friday 24 July 2026, mid-afternoon local time.
const NOW = new Date(2026, 6, 24, 15, 30, 0)

function renderPicker(
  overrides: Partial<React.ComponentProps<typeof DateRangePicker>> = {}
) {
  const onChange = jest.fn()
  const view = render(
    <DateRangePicker value={{}} onChange={onChange} now={NOW} {...overrides} />
  )
  return { onChange, ...view }
}

const openPopover = async () => {
  await userEvent.click(
    screen.getByRole('button', { name: /date range|Pick a/i })
  )
}

/** rdp labels each day button "Weekday, Month Nth, Year". */
const day = (label: string) => screen.getByRole('button', { name: label })

it('shows the placeholder while nothing is selected', () => {
  renderPicker({ placeholder: 'Any date' })

  expect(screen.getByRole('button', { name: 'Any date' })).toBeInTheDocument()
})

it('emits a local-time range for a preset and labels it by name', async () => {
  const { onChange } = renderPicker()
  await openPopover()

  await userEvent.click(screen.getByRole('button', { name: 'Last 30 days' }))

  expect(onChange).toHaveBeenCalledTimes(1)
  const emitted = onChange.mock.calls[0][0] as DateRangeValue
  // 30 days *including* today: 25 June → 24 July.
  expect(emitted.from).toEqual(new Date(2026, 5, 25, 0, 0, 0, 0))
  expect(emitted.to).toEqual(new Date(2026, 6, 24, 23, 59, 59, 999))
})

it('round-trips a controlled value back into the trigger label', async () => {
  const { onChange, rerender } = renderPicker()
  await openPopover()
  await userEvent.click(screen.getByRole('button', { name: 'Last 7 days' }))

  const emitted = onChange.mock.calls[0][0] as DateRangeValue
  rerender(<DateRangePicker value={emitted} onChange={onChange} now={NOW} />)

  // Recognised as the preset it came from, not spelled out as two dates.
  expect(
    screen.getByRole('button', { name: 'Date range: Last 7 days' })
  ).toBeInTheDocument()
})

it('spells out a range that matches no preset', () => {
  renderPicker({
    value: { from: new Date(2026, 6, 1), to: new Date(2026, 6, 24) },
  })

  expect(
    screen.getByRole('button', { name: 'Date range: 1 Jul – 24 Jul 2026' })
  ).toBeInTheDocument()
})

it('selects a range from two calendar clicks', async () => {
  const { onChange, rerender } = renderPicker()
  await openPopover()

  await userEvent.click(day('Monday, July 6th, 2026'))
  const afterFirst = onChange.mock.calls[0][0] as DateRangeValue
  // react-day-picker treats the opening click as a one-day range (from === to),
  // not as a half-open one, so the trigger reads as a single date until the
  // second click widens it.
  expect(afterFirst.from).toEqual(new Date(2026, 6, 6))
  expect(afterFirst.to).toEqual(new Date(2026, 6, 6))

  // Controlled: the second click only extends the range if the parent fed the
  // first one back, which is exactly what a consumer does.
  rerender(<DateRangePicker value={afterFirst} onChange={onChange} now={NOW} />)
  await userEvent.click(day('Saturday, July 11th, 2026'))

  const afterSecond = onChange.mock.calls[1][0] as DateRangeValue
  expect(afterSecond.from).toEqual(new Date(2026, 6, 6))
  expect(afterSecond.to).toEqual(new Date(2026, 6, 11))
})

it('clears the range and closes', async () => {
  const { onChange } = renderPicker({
    value: { from: new Date(2026, 6, 1), to: new Date(2026, 6, 24) },
  })
  await openPopover()

  await userEvent.click(screen.getByRole('button', { name: 'Clear' }))

  expect(onChange).toHaveBeenCalledWith({})
})

it('disables Clear when there is nothing to clear', async () => {
  renderPicker()
  await openPopover()

  expect(screen.getByRole('button', { name: 'Clear' })).toBeDisabled()
})

it('marks the active preset so the current choice is visible', async () => {
  renderPicker({ value: rangeForPreset(DEFAULT_RANGE_PRESETS[1], NOW) })
  await openPopover()

  const active = screen.getByRole('button', { name: 'Last 30 days' })
  const inactive = screen.getByRole('button', { name: 'Last 7 days' })
  // The variants differ; asserting on the class the `secondary` variant adds
  // rather than on a colour keeps this tied to the design tokens.
  expect(active.className).toContain('bg-secondary')
  expect(inactive.className).not.toContain('bg-secondary')
})

it('opens on the month the range starts in, not on today', async () => {
  renderPicker({
    value: { from: new Date(2026, 1, 3), to: new Date(2026, 1, 20) },
  })
  await openPopover()

  // February, restored from the value — not July, which is `now`.
  expect(
    screen.getByRole('grid', { name: 'February 2026' })
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('grid', { name: 'July 2026' })
  ).not.toBeInTheDocument()
})

it('renders no preset column when presets are empty', async () => {
  renderPicker({ presets: [] })
  await openPopover()

  expect(
    screen.queryByRole('button', { name: 'Last 7 days' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('grid', { name: 'July 2026' })).toBeInTheDocument()
})

it('honours an explicit accessible name', () => {
  renderPicker({ ariaLabel: 'Created between' })

  expect(
    screen.getByRole('button', { name: 'Created between' })
  ).toBeInTheDocument()
})

it('closes on Escape and returns focus to the trigger', async () => {
  renderPicker()
  const trigger = screen.getByRole('button', { name: /Pick a date range/ })
  await userEvent.click(trigger)
  expect(screen.getByRole('grid', { name: 'July 2026' })).toBeInTheDocument()

  await userEvent.keyboard('{Escape}')

  expect(
    screen.queryByRole('grid', { name: 'July 2026' })
  ).not.toBeInTheDocument()
  expect(trigger).toHaveFocus()
})

it('opens from the keyboard alone', async () => {
  renderPicker()

  await userEvent.tab()
  expect(
    screen.getByRole('button', { name: /Pick a date range/ })
  ).toHaveFocus()
  await userEvent.keyboard('{Enter}')

  expect(screen.getByRole('grid', { name: 'July 2026' })).toBeInTheDocument()
})

it('does not open while disabled', async () => {
  renderPicker({ disabled: true })

  await userEvent.click(
    screen.getByRole('button', { name: /Pick a date range/ })
  )

  expect(screen.queryByRole('grid')).not.toBeInTheDocument()
})

describe('a controlled value the picker itself would never produce', () => {
  it('orders an inverted range rather than showing nothing', () => {
    renderPicker({
      // A consumer restoring two dates from a URL has no ordering guarantee.
      value: { from: new Date(2026, 6, 24), to: new Date(2026, 6, 1) },
    })

    expect(
      screen.getByRole('button', { name: 'Date range: 1 Jul – 24 Jul 2026' })
    ).toBeInTheDocument()
  })

  it('labels a half-open range by its single date', () => {
    renderPicker({ value: { from: new Date(2026, 6, 4) } })

    expect(
      screen.getByRole('button', { name: 'Date range: 4 Jul 2026' })
    ).toBeInTheDocument()
  })
})

describe('local time, not UTC', () => {
  // The bug this guards is documented on `parseLocalDate` in
  // TimeSeriesBarChart.tsx: `new Date('2026-05-01')` parses as UTC, so anyone
  // west of it renders the previous day. These assertions read local getters, so
  // they hold whatever timezone the runner is in.
  it('pins preset boundaries to local midnight and local end-of-day', () => {
    const range = rangeForPreset({ label: 'Last 7 days', days: 7 }, NOW)

    expect(range.from?.getHours()).toBe(0)
    expect(range.from?.getMinutes()).toBe(0)
    expect(range.from?.getDate()).toBe(18)
    expect(range.from?.getMonth()).toBe(6)
    expect(range.to?.getHours()).toBe(23)
    expect(range.to?.getMinutes()).toBe(59)
    expect(range.to?.getDate()).toBe(24)
  })

  it('formats the calendar day the Date actually names', () => {
    // Local midnight on the 1st: a UTC-based formatter would print 30 Apr for
    // any negative-offset zone.
    const firstOfMay = new Date(2026, 4, 1, 0, 0, 0)

    expect(formatRangeLabel({ from: firstOfMay, to: firstOfMay })).toBe(
      '1 May 2026'
    )
  })

  it('treats a same-day range as one date', () => {
    const d = new Date(2026, 6, 4, 9, 0)

    expect(formatRangeLabel({ from: d, to: new Date(2026, 6, 4, 23, 0) })).toBe(
      '4 Jul 2026'
    )
  })
})

it('drives a parent that holds the value in state', async () => {
  function Host() {
    const [range, setRange] = useState<DateRangeValue>({})
    return (
      <div>
        <DateRangePicker value={range} onChange={setRange} now={NOW} />
        <output data-testid="held">
          {range.from ? range.from.toDateString() : 'empty'}
        </output>
      </div>
    )
  }
  render(<Host />)

  await openPopover()
  await userEvent.click(screen.getByRole('button', { name: 'Last 90 days' }))

  // 90 days including 24 July 2026 reaches back to 26 April.
  expect(screen.getByTestId('held')).toHaveTextContent('Sun Apr 26 2026')
  // The popover closed on a preset, and the trigger now names the choice.
  expect(
    screen.getByRole('button', { name: 'Date range: Last 90 days' })
  ).toBeInTheDocument()
})

it('keeps the two months adjacent so a range can span them', async () => {
  renderPicker()
  await openPopover()

  const grids = screen.getAllByRole('grid')
  expect(grids).toHaveLength(2)
  expect(grids[0]).toHaveAccessibleName('July 2026')
  expect(grids[1]).toHaveAccessibleName('August 2026')
  // Sanity: a day from the second month is reachable without paging.
  expect(
    within(grids[1]).getByRole('button', { name: 'Monday, August 3rd, 2026' })
  ).toBeInTheDocument()
})

describe('the default clock', () => {
  // Every test above injects `now`, which would leave the real-runtime path —
  // where the clock defaults to the system time — entirely unexercised.
  it('defaults to the system clock in rangeForPreset', () => {
    const today = new Date()
    const range = rangeForPreset({ label: 'Today', days: 1 })

    expect(range.from?.getDate()).toBe(today.getDate())
    expect(range.from?.getHours()).toBe(0)
    expect(range.to?.getDate()).toBe(today.getDate())
    expect(range.to?.getHours()).toBe(23)
  })

  it('defaults to the system clock in the picker', () => {
    const range = rangeForPreset(DEFAULT_RANGE_PRESETS[0])
    render(<DateRangePicker value={range} onChange={jest.fn()} />)

    // The label resolves against the same default clock, so a preset-shaped
    // value is still recognised as that preset.
    expect(
      screen.getByRole('button', { name: 'Date range: Last 7 days' })
    ).toBeInTheDocument()
  })

  it('defaults to the system clock in formatRangeLabel', () => {
    const range = rangeForPreset(DEFAULT_RANGE_PRESETS[2])

    expect(formatRangeLabel(range)).toBe('Last 90 days')
  })
})

describe('URL params (#460)', () => {
  it('formats a Date as its local calendar day', () => {
    expect(toDateParam(new Date(2026, 6, 4, 23, 30))).toBe('2026-07-04')
    // Late-evening local time is still that local day, even when the UTC day has
    // already rolled over for eastern offsets.
    expect(toDateParam(new Date(2026, 0, 1, 0, 0))).toBe('2026-01-01')
  })

  it('parses a param to exactly local midnight of that day', () => {
    // Compared against a component-built Date, which is the only assertion that
    // distinguishes local construction from `new Date('2026-07-01')` — the latter
    // is parsed as UTC and is a different instant in every non-UTC zone.
    expect(fromDateParam('2026-07-01')).toEqual(new Date(2026, 6, 1))
    expect(fromDateParam('2026-01-31')).toEqual(new Date(2026, 0, 31))
  })

  it('round-trips a Date through the param and back', () => {
    const day = new Date(2026, 6, 4)

    expect(fromDateParam(toDateParam(day))).toEqual(day)
  })

  it('rejects anything that is not a real YYYY-MM-DD day', () => {
    expect(fromDateParam(undefined)).toBeUndefined()
    expect(fromDateParam('')).toBeUndefined()
    expect(fromDateParam('not-a-date')).toBeUndefined()
    expect(fromDateParam('2026-7-1')).toBeUndefined()
    expect(fromDateParam('2026-07-01T10:00:00Z')).toBeUndefined()
    // Well-shaped but impossible: JS would roll this forward into March.
    expect(fromDateParam('2026-02-31')).toBeUndefined()
  })

  it('bounds a range at local midnight and local end-of-day', () => {
    const instants = rangeToInstants({
      from: new Date(2026, 6, 1),
      to: new Date(2026, 6, 4),
    })

    expect(instants.from).toBe(new Date(2026, 6, 1, 0, 0, 0, 0).toISOString())
    // End-of-day, not midnight: a bare midnight upper bound excludes everything
    // that happened during the last day, so a one-day filter returns nothing.
    expect(instants.to).toBe(
      new Date(2026, 6, 4, 23, 59, 59, 999).toISOString()
    )
  })

  it('orders an inverted range before converting', () => {
    const instants = rangeToInstants({
      from: new Date(2026, 6, 4),
      to: new Date(2026, 6, 1),
    })

    expect(instants.from).toBe(new Date(2026, 6, 1, 0, 0, 0, 0).toISOString())
    expect(instants.to).toBe(
      new Date(2026, 6, 4, 23, 59, 59, 999).toISOString()
    )
  })

  it('leaves an absent bound absent', () => {
    expect(rangeToInstants({})).toEqual({ from: undefined, to: undefined })
    expect(rangeToInstants({ from: new Date(2026, 6, 1) }).to).toBeUndefined()
  })
})
