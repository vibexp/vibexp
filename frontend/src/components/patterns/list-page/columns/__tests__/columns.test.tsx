import type { ColumnDef, Row } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'

import {
  fieldOfRole,
  getResourceDescriptor,
  statusFieldOf,
} from '@/components/patterns/resource'

import {
  actionsColumn,
  columnList,
  nameColumn,
  statusColumn,
  taxonomyColumn,
  typeColumn,
  updatedColumn,
} from '..'

/*
 * The factories are driven by the REAL descriptors from the registry, so a
 * descriptor change (a status value losing its tone, a `name` field renamed)
 * fails here first rather than in five page suites.
 */
const prompt = getResourceDescriptor('prompt')
const artifact = getResourceDescriptor('artifact')
const blueprint = getResourceDescriptor('blueprint')
const memory = getResourceDescriptor('memory')
const galleryPrompt = getResourceDescriptor('gallery-prompt')

interface TestRow {
  id: string
  name: string
  status: string
  type: string
  labels: string[]
  updated_at: string
  description?: string
}

const row = (overrides: Partial<TestRow> = {}): TestRow => ({
  id: 'r1',
  name: 'Runbook',
  status: 'draft',
  type: 'general',
  labels: [],
  updated_at: '2024-01-15T12:00:00Z',
  ...overrides,
})

/** Renders a column's cell the way `<ListTable>` does — `row.original` only. */
function renderCell(col: ColumnDef<TestRow> | undefined, item: TestRow) {
  if (!col?.cell || typeof col.cell !== 'function') {
    throw new Error('column has no cell renderer')
  }
  const cell = col.cell as (ctx: { row: Row<TestRow> }) => ReactNode
  return render(<>{cell({ row: { original: item } as Row<TestRow> })}</>)
}

describe('columnList', () => {
  it('drops the columns a descriptor cannot describe', () => {
    const kept: ColumnDef<TestRow> = { id: 'kept' }
    expect(columnList<TestRow>(kept, undefined, false, null)).toEqual([kept])
  })
})

describe('nameColumn', () => {
  it('takes its accessorKey and header from the descriptor field', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(memory, 'name'),
      value: r => r.name,
    })
    expect(col).toMatchObject({ accessorKey: 'text', header: 'Memory' })
  })

  it('accepts a header override for a list that titles the column differently', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(memory, 'name'),
      header: 'Content',
      value: r => r.name,
    })
    expect(col).toMatchObject({ header: 'Content' })
  })

  it('renders a link that navigates to the row when `to` is given', async () => {
    const navigate = vi.fn()
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
      to: r => `/prompts/${r.id}`,
      navigate,
    })
    renderCell(col, row())
    await userEvent.click(screen.getByRole('button', { name: 'Runbook' }))
    expect(navigate.mock.calls).toEqual([['/prompts/r1']])
  })

  it('renders plain text when no route is given', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
    })
    renderCell(col, row())
    expect(screen.getByText('Runbook')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders the summary line and the per-resource adornment', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
      summary: r => r.description,
      adornment: () => <span data-testid="adornment" />,
    })
    renderCell(col, row({ description: 'A short description' }))
    expect(screen.getByText('A short description')).toBeInTheDocument()
    expect(screen.getByTestId('adornment')).toBeInTheDocument()
  })

  it('returns undefined for a descriptor with no name field', () => {
    expect(
      nameColumn<TestRow>({ field: undefined, value: r => r.name })
    ).toBeUndefined()
  })
})

describe('typeColumn', () => {
  it('labels the value from the descriptor', () => {
    const col = typeColumn<TestRow>({
      field: fieldOfRole(blueprint, 'type'),
      value: r => r.type,
    })
    expect(col).toMatchObject({ accessorKey: 'type', header: 'Type' })
    renderCell(col, row({ type: 'claude-code' }))
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
  })

  it('honours a label override for team-defined types', () => {
    const col = typeColumn<TestRow>({
      field: fieldOfRole(artifact, 'type'),
      value: r => r.type,
      label: slug => `Team: ${slug}`,
    })
    renderCell(col, row({ type: 'runbooks' }))
    expect(screen.getByText('Team: runbooks')).toBeInTheDocument()
  })

  it('returns undefined for a kind with no type field', () => {
    expect(
      typeColumn<TestRow>({
        field: fieldOfRole(prompt, 'type'),
        value: r => r.type,
      })
    ).toBeUndefined()
  })
})

describe('statusColumn', () => {
  it('renders the descriptor tone and label as a StatusBadge', () => {
    const col = statusColumn<TestRow>({
      field: statusFieldOf(artifact),
      value: r => r.status,
    })
    expect(col).toMatchObject({ accessorKey: 'status', header: 'Status' })
    renderCell(col, row({ status: 'draft' }))
    const badge = screen.getByText('Draft')
    expect(badge).toHaveClass('bg-warning')
  })

  it('falls back to a neutral badge for a value the descriptor has no tone for', () => {
    const col = statusColumn<TestRow>({
      field: statusFieldOf(blueprint),
      value: r => r.status,
    })
    renderCell(col, row({ status: 'quarantined' }))
    const badge = screen.getByText('quarantined')
    expect(badge).toHaveClass('bg-muted')
  })

  it('returns undefined for a kind with no status field', () => {
    expect(
      statusColumn<TestRow>({
        field: statusFieldOf(galleryPrompt),
        value: r => r.status,
      })
    ).toBeUndefined()
  })
})

describe('taxonomyColumn', () => {
  it('takes its id and header from the descriptor field', () => {
    const col = taxonomyColumn<TestRow>({
      field: fieldOfRole(prompt, 'taxonomy'),
      values: r => r.labels,
    })
    expect(col).toMatchObject({ id: 'labels', header: 'Labels' })
  })

  it('renders chips with a +N overflow badge past the cap', () => {
    const col = taxonomyColumn<TestRow>({
      field: fieldOfRole(prompt, 'taxonomy'),
      values: r => r.labels,
    })
    renderCell(col, row({ labels: ['a', 'b', 'c', 'd', 'e'] }))
    expect(screen.getByText('c')).toBeInTheDocument()
    expect(screen.queryByText('d')).not.toBeInTheDocument()
    expect(screen.getByText('+2')).toBeInTheDocument()
  })

  it('renders an em dash when there is nothing to group by', () => {
    const col = taxonomyColumn<TestRow>({
      field: fieldOfRole(prompt, 'taxonomy'),
      values: r => r.labels,
    })
    renderCell(col, row({ labels: [] }))
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

describe('updatedColumn', () => {
  it('defaults to the updated_at accessor', () => {
    expect(updatedColumn<TestRow>({ value: r => r.updated_at })).toMatchObject({
      accessorKey: 'updated_at',
      header: 'Updated',
    })
  })

  it('can be reused for another timestamp', () => {
    expect(
      updatedColumn<TestRow>({
        value: r => r.updated_at,
        accessorKey: 'last_run',
        header: 'Last run',
      })
    ).toMatchObject({ accessorKey: 'last_run', header: 'Last run' })
  })

  it('renders a relative label with the absolute time in `title`', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2024-06-01T12:00:00Z'))
    const col = updatedColumn<TestRow>({ value: r => r.updated_at })
    renderCell(col, row({ updated_at: '2024-06-01T09:00:00Z' }))
    const label = screen.getByText('3h ago')
    expect(label).toHaveAttribute('title', expect.stringContaining('2024'))
    expect(label.getAttribute('title')).not.toBe(label.textContent)
    vi.useRealTimers()
  })
})

describe('actionsColumn', () => {
  const del = vi.fn()

  beforeEach(() => {
    del.mockClear()
  })

  function deleteAction(visible?: (r: TestRow) => boolean) {
    return actionsColumn<TestRow>({
      singular: 'prompt',
      actions: [
        {
          key: 'delete',
          label: 'Delete',
          icon: Trash2,
          onSelect: del,
          visible,
        },
      ],
    })
  }

  it('is the `actions` column ListTable knows to right-align', () => {
    expect(deleteAction()).toMatchObject({ id: 'actions' })
  })

  it('gives every button an `<action>-<singular>-button` test id', async () => {
    renderCell(deleteAction(), row())
    const button = screen.getByTestId('delete-prompt-button')
    expect(button).toHaveAttribute('aria-label', 'Delete')
    await userEvent.click(button)
    expect(del.mock.calls).toHaveLength(1)
  })

  it('hides an action the row is not permitted', () => {
    renderCell(
      deleteAction(() => false),
      row()
    )
    expect(screen.queryByTestId('delete-prompt-button')).not.toBeInTheDocument()
  })

  it('interpolates a per-row accessible name', () => {
    const col = actionsColumn<TestRow>({
      singular: 'agent',
      actions: [
        {
          key: 'delete',
          label: r => `Delete ${r.name}`,
          icon: Trash2,
          onSelect: del,
        },
      ],
    })
    renderCell(col, row())
    expect(screen.getByTestId('delete-agent-button')).toHaveAttribute(
      'aria-label',
      'Delete Runbook'
    )
  })
})
