import '@testing-library/jest-dom'

import type { ColumnDef } from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ListTable } from './ListTable'

interface Row {
  id: string
  name: string
  status: 'draft' | 'published'
  count: number
}

const ROWS: Row[] = [
  { id: '1', name: 'Alpha', status: 'draft', count: 5 },
  { id: '2', name: 'Bravo', status: 'published', count: 12 },
]

const BASIC_COLUMNS: ColumnDef<Row>[] = [
  {
    accessorKey: 'name',
    header: 'Name',
    cell: ({ row }) => <span>{row.original.name}</span>,
  },
  {
    accessorKey: 'status',
    header: 'Status',
    cell: ({ row }) => <span>{row.original.status}</span>,
  },
  {
    accessorKey: 'count',
    header: 'Count',
    meta: { align: 'right' },
    cell: ({ row }) => <span>{row.original.count}</span>,
  },
  {
    id: 'actions',
    cell: ({ row }) => (
      <button data-testid={`action-${row.original.id}`}>Edit</button>
    ),
  },
]

describe('<ListTable> sort behavior', () => {
  type SortKey = 'name' | 'status'
  const SORTABLE: readonly SortKey[] = ['name', 'status']

  it('marks sortable headers with role="button", tabIndex=0, and aria-sort="none" by default', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        onSortChange={jest.fn()}
      />
    )

    const nameHeader = screen.getByRole('button', { name: /Name/ })
    expect(nameHeader).toHaveAttribute('aria-sort', 'none')
    expect(nameHeader).toHaveAttribute('tabindex', '0')
  })

  it('non-sortable headers get no role, tabIndex, or aria-sort', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        onSortChange={jest.fn()}
      />
    )

    // Count column is not in SORTABLE
    const countHeader = screen.getByText('Count').closest('th')
    expect(countHeader).not.toBeNull()
    expect(countHeader).not.toHaveAttribute('role', 'button')
    expect(countHeader).not.toHaveAttribute('tabindex')
    expect(countHeader).not.toHaveAttribute('aria-sort')
  })

  it('clicking a sortable header invokes onSortChange with the accessor key', async () => {
    const user = userEvent.setup()
    const onSortChange = jest.fn()

    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        onSortChange={onSortChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /Name/ }))
    expect(onSortChange).toHaveBeenCalledWith('name')
  })

  it('pressing Enter on a sortable header invokes onSortChange', async () => {
    const user = userEvent.setup()
    const onSortChange = jest.fn()

    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        onSortChange={onSortChange}
      />
    )

    const nameHeader = screen.getByRole('button', { name: /Name/ })
    nameHeader.focus()
    await user.keyboard('{Enter}')
    expect(onSortChange).toHaveBeenCalledWith('name')
  })

  it('pressing Space on a sortable header invokes onSortChange', async () => {
    const user = userEvent.setup()
    const onSortChange = jest.fn()

    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        onSortChange={onSortChange}
      />
    )

    const statusHeader = screen.getByRole('button', { name: /Status/ })
    statusHeader.focus()
    await user.keyboard(' ')
    expect(onSortChange).toHaveBeenCalledWith('status')
  })

  it('aria-sort becomes "ascending" when sortKey matches and sortDir is asc', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        sortKey="name"
        sortDir="asc"
        onSortChange={jest.fn()}
      />
    )

    expect(screen.getByRole('button', { name: /Name/ })).toHaveAttribute(
      'aria-sort',
      'ascending'
    )
  })

  it('aria-sort becomes "descending" when sortKey matches and sortDir is desc', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        sortKey="status"
        sortDir="desc"
        onSortChange={jest.fn()}
      />
    )

    expect(screen.getByRole('button', { name: /Status/ })).toHaveAttribute(
      'aria-sort',
      'descending'
    )
  })

  it('shows ChevronUp icon when active and sortDir is asc', () => {
    const { container } = render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        sortKey="name"
        sortDir="asc"
        onSortChange={jest.fn()}
      />
    )

    const nameHeader = screen.getByRole('button', { name: /Name/ })
    expect(within(nameHeader).getByTestId('chevronup-icon')).toBeInTheDocument()
    // Inactive headers show the neutral chevron
    expect(
      container.querySelectorAll('[data-testid="chevronsupdown-icon"]').length
    ).toBeGreaterThan(0)
  })

  it('shows ChevronDown icon when active and sortDir is desc', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
        sortKey="name"
        sortDir="desc"
        onSortChange={jest.fn()}
      />
    )

    const nameHeader = screen.getByRole('button', { name: /Name/ })
    expect(
      within(nameHeader).getByTestId('chevrondown-icon')
    ).toBeInTheDocument()
  })

  it('disables sortability when no onSortChange handler is provided', () => {
    render(
      <ListTable<Row, SortKey>
        rows={ROWS}
        columns={BASIC_COLUMNS}
        sortableKeys={SORTABLE}
      />
    )

    expect(
      screen.queryByRole('button', { name: /Name/ })
    ).not.toBeInTheDocument()
  })
})

describe('<ListTable> row interactions', () => {
  it('calls onRowClick when a row is clicked', async () => {
    const user = userEvent.setup()
    const onRowClick = jest.fn()

    render(
      <ListTable rows={ROWS} columns={BASIC_COLUMNS} onRowClick={onRowClick} />
    )

    // Click the row by clicking a non-actions cell
    await user.click(screen.getByText('Alpha'))
    expect(onRowClick).toHaveBeenCalledTimes(1)
    expect(onRowClick).toHaveBeenCalledWith(ROWS[0])
  })

  it('rows have cursor-pointer class when onRowClick is provided', () => {
    render(
      <ListTable rows={ROWS} columns={BASIC_COLUMNS} onRowClick={jest.fn()} />
    )

    const row = screen.getByText('Alpha').closest('tr')
    expect(row).not.toBeNull()
    expect(row?.className).toContain('cursor-pointer')
  })

  it('rows do NOT have cursor-pointer class when onRowClick is omitted', () => {
    render(<ListTable rows={ROWS} columns={BASIC_COLUMNS} />)

    const row = screen.getByText('Alpha').closest('tr')
    expect(row?.className).not.toContain('cursor-pointer')
  })

  it('clicking inside the actions column does NOT trigger onRowClick', async () => {
    const user = userEvent.setup()
    const onRowClick = jest.fn()

    render(
      <ListTable rows={ROWS} columns={BASIC_COLUMNS} onRowClick={onRowClick} />
    )

    await user.click(screen.getByTestId('action-1'))
    expect(onRowClick).not.toHaveBeenCalled()
  })
})

describe('<ListTable> column meta.align', () => {
  it('applies text-right to header and body cells when meta.align is "right"', () => {
    render(<ListTable rows={ROWS} columns={BASIC_COLUMNS} />)

    // Count column has meta: { align: 'right' }
    const countHeader = screen.getByText('Count').closest('th')
    expect(countHeader?.className).toContain('text-right')

    // Find the body cell containing "5" (count for first row)
    const countCell = screen.getByText('5').closest('td')
    expect(countCell?.className).toContain('text-right')

    // Sanity: a column without meta.align should not have text-right
    const nameHeader = screen.getByText('Name').closest('th')
    expect(nameHeader?.className).not.toContain('text-right')
  })

  it('actions column header is auto-aligned right (built-in behavior)', () => {
    render(<ListTable rows={ROWS} columns={BASIC_COLUMNS} />)

    // The actions column has no `header`, so we find by role and position
    const headers = screen.getAllByRole('columnheader')
    const actionsHeader = headers[headers.length - 1]
    expect(actionsHeader.className).toContain('text-right')
  })
})

describe('<ListTable> rendering edge cases', () => {
  it('renders an em-dash for cells without a cell renderer', () => {
    const cols: ColumnDef<Row>[] = [
      { accessorKey: 'name', header: 'Name' }, // no `cell` defined
    ]

    render(<ListTable rows={[ROWS[0]]} columns={cols} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders an empty body when rows is an empty array', () => {
    render(<ListTable rows={[]} columns={BASIC_COLUMNS} />)

    // Headers still render
    expect(screen.getByText('Name')).toBeInTheDocument()
    // No data rows
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })
})
