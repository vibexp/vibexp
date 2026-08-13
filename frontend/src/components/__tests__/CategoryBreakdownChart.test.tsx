import { render, screen } from '@testing-library/react'

import {
  CategoryBreakdownChart,
  type CategoryDatum,
} from '../CategoryBreakdownChart'

const rows = (...counts: number[]): CategoryDatum[] =>
  counts.map((count, i) => ({
    key: `k${String(i)}`,
    label: `Category ${String(i)}`,
    count,
  }))

const renderChart = (
  props: Partial<React.ComponentProps<typeof CategoryBreakdownChart>> = {}
) =>
  render(
    <CategoryBreakdownChart
      title="Stale by type"
      totalLabel="Stale resources"
      total={10}
      data={rows(6, 4)}
      loading={false}
      error={false}
      errorMessage="Couldn't load it"
      emptyMessage="Nothing is stale right now."
      {...props}
    />
  )

describe('CategoryBreakdownChart', () => {
  it('renders one row per category with its count', () => {
    renderChart()

    expect(screen.getAllByTestId('category-row')).toHaveLength(2)
    expect(screen.getByText('Category 0')).toBeInTheDocument()
    expect(screen.getByText('6')).toBeInTheDocument()
  })

  it('sorts rows high to low regardless of input order', () => {
    renderChart({
      data: [
        { key: 'a', label: 'Small', count: 1 },
        { key: 'b', label: 'Big', count: 9 },
      ],
    })

    const labels = screen
      .getAllByTestId('category-row')
      .map(row => row.textContent)
    expect(labels[0]).toContain('Big')
    expect(labels[1]).toContain('Small')
  })

  it('shows the headline total', () => {
    renderChart({ total: 42 })
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('caps the list and reports how many were hidden', () => {
    // by-project and by-rule return an entry for EVERY project/rule, zeros
    // included, so the list must not grow with the workspace.
    renderChart({ data: rows(9, 8, 7, 6, 5, 4, 3, 2, 1), maxRows: 3 })

    expect(screen.getAllByTestId('category-row')).toHaveLength(3)
    expect(screen.getByText('+6 more')).toBeInTheDocument()
  })

  it('omits the overflow line when everything fits', () => {
    renderChart({ data: rows(3, 2), maxRows: 5 })
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument()
  })

  it('renders the empty message when nothing is stale', () => {
    renderChart({ total: 0, data: [] })

    expect(screen.getByText('Nothing is stale right now.')).toBeInTheDocument()
    expect(screen.queryAllByTestId('category-row')).toHaveLength(0)
  })

  it('treats a zero total as empty even when categories are present', () => {
    // Every category reporting 0 is "nothing is stale", not a chart of nothing.
    renderChart({ total: 0, data: rows(0, 0) })
    expect(screen.getByText('Nothing is stale right now.')).toBeInTheDocument()
  })

  it('renders the error message instead of the rows', () => {
    renderChart({ error: true })

    expect(screen.getByText("Couldn't load it")).toBeInTheDocument()
    expect(screen.queryAllByTestId('category-row')).toHaveLength(0)
  })

  it('renders neither rows nor messages while loading', () => {
    renderChart({ loading: true })

    expect(screen.queryAllByTestId('category-row')).toHaveLength(0)
    expect(
      screen.queryByText('Nothing is stale right now.')
    ).not.toBeInTheDocument()
  })

  it('renders a row detail suffix when given', () => {
    renderChart({
      data: [
        { key: 'r', label: 'Artifacts', count: 3, detail: '90d · disabled' },
      ],
    })
    expect(screen.getByText('90d · disabled')).toBeInTheDocument()
  })
})
