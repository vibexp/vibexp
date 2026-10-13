import type { ColumnDef, Row } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

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
  last_write_source: 'web' | 'cli' | 'mcp' | 'api' | null
  last_write_assistant_name: string | null
}

const row = (overrides: Partial<TestRow> = {}): TestRow => ({
  id: 'r1',
  name: 'Runbook',
  status: 'draft',
  type: 'general',
  labels: [],
  updated_at: '2024-01-15T12:00:00Z',
  last_write_source: null,
  last_write_assistant_name: null,
  ...overrides,
})

/** Renders a column's cell the way `<ListTable>` does — `row.original` only. */
function renderCell(col: ColumnDef<TestRow> | undefined, item: TestRow) {
  if (!col?.cell || typeof col.cell !== 'function') {
    throw new Error('column has no cell renderer')
  }
  const cell = col.cell as (ctx: { row: Row<TestRow> }) => ReactNode
  // In a router because a linked name is a real `<a href>` (react-router Link).
  return render(
    <MemoryRouter>
      {cell({ row: { original: item } as Row<TestRow> })}
    </MemoryRouter>
  )
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
    expect(col).toMatchObject({ accessorKey: 'title', header: 'Title' })
  })

  it('accepts a header override for a list that titles the column differently', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(memory, 'name'),
      header: 'Content',
      value: r => r.name,
    })
    expect(col).toMatchObject({ header: 'Content' })
  })

  it('renders a real link to the row when `to` is given', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
      to: r => `/prompts/${r.id}`,
    })
    renderCell(col, row())
    // An anchor with an href — not a scripted button — is what makes
    // middle-click, Ctrl/Cmd-click and "open in new tab" work.
    expect(screen.getByRole('link', { name: 'Runbook' })).toHaveAttribute(
      'href',
      '/prompts/r1'
    )
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('draws a stand-in name subdued', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
      to: r => `/prompts/${r.id}`,
      subdued: r => r.id === 'r2',
    })
    renderCell(col, row())
    expect(screen.getByRole('link', { name: 'Runbook' })).not.toHaveClass(
      'text-muted-foreground'
    )
    renderCell(col, row({ id: 'r2', name: 'Stand-in' }))
    expect(screen.getByRole('link', { name: 'Stand-in' })).toHaveClass(
      'text-muted-foreground'
    )
  })

  it('renders plain text when no route is given', () => {
    const col = nameColumn<TestRow>({
      field: fieldOfRole(prompt, 'name'),
      value: r => r.name,
    })
    renderCell(col, row())
    expect(screen.getByText('Runbook')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
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

describe('nameColumn, with the last write', () => {
  const col = nameColumn<TestRow>({
    field: fieldOfRole(prompt, 'name'),
    value: r => r.name,
    summary: r => r.description,
    lastWrite: r => r,
  })
  const indicator = () => screen.queryByTestId('last-write-indicator')

  it('names the channel and the assistant after the summary, on its line', () => {
    renderCell(
      col,
      row({
        description: 'How to deploy',
        last_write_source: 'mcp',
        last_write_assistant_name: 'Claude Code',
      })
    )
    expect(indicator()).toHaveTextContent(/^via MCP · Claude Code$/)
    expect(indicator()?.parentElement).toHaveTextContent(
      /^How to deploy·via MCP · Claude Code$/
    )
  })

  it.each([
    ['mcp', 'via MCP'],
    ['cli', 'via CLI'],
    ['api', 'via API'],
  ] as const)('names a %s write on a row with no summary', (source, text) => {
    renderCell(col, row({ last_write_source: source }))
    expect(indicator()?.parentElement?.textContent).toBe(text)
  })

  it.each([['web'], [null]] as const)(
    'adds nothing for a %s last write',
    source => {
      const { container } = renderCell(
        col,
        row({ last_write_source: source, last_write_assistant_name: 'X' })
      )
      expect(indicator()).not.toBeInTheDocument()
      // No summary either, so no second line at all.
      expect(container.querySelector('p')).not.toBeInTheDocument()
    }
  )

  it('leaves the summary alone for a web write', () => {
    renderCell(
      col,
      row({ description: 'How to deploy', last_write_source: 'web' })
    )
    expect(screen.getByText('How to deploy').parentElement).toHaveTextContent(
      /^How to deploy$/
    )
  })

  it('never wraps: the summary and a long assistant name truncate', () => {
    renderCell(
      col,
      row({
        description: 'How to deploy',
        last_write_source: 'mcp',
        last_write_assistant_name: 'An assistant name of thirty ch',
      })
    )
    expect(screen.getByText('How to deploy')).toHaveClass('truncate')
    expect(indicator()).toHaveClass('truncate', 'shrink-0')
    expect(screen.getByText('An assistant name of thirty ch')).toHaveAttribute(
      'title',
      'An assistant name of thirty ch'
    )
  })

  it('shows no last write on a list that passes none', () => {
    renderCell(
      nameColumn<TestRow>({
        field: fieldOfRole(prompt, 'name'),
        value: r => r.name,
      }),
      row({ last_write_source: 'mcp' })
    )
    expect(indicator()).not.toBeInTheDocument()
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

  // A team's custom status (#1263) has a display name only the catalog knows;
  // the page passes `useStatusOptions(descriptor).labelOf`.
  it('words the badge through `label` when the page supplies one', () => {
    const col = statusColumn<TestRow>({
      field: statusFieldOf(artifact),
      value: r => r.status,
      label: raw => (raw === 'in-review' ? 'In review' : raw),
    })
    renderCell(col, row({ status: 'in-review' }))
    expect(screen.getByText('In review')).toHaveClass('bg-muted')
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
