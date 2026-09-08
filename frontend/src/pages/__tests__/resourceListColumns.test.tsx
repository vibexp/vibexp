import type { ColumnDef } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'

import { ListTable } from '@/components/patterns/list-page'
import { buildAgentsColumns } from '@/pages/agents/agentsColumns'
import { buildArtifactsColumns } from '@/pages/artifacts/artifactsColumns'
import { buildBlueprintsColumns } from '@/pages/blueprints/blueprintsColumns'
import { buildMemoriesColumns } from '@/pages/memories/memoriesColumns'
import { buildPromptsColumns } from '@/pages/prompts/promptsColumns'

/*
 * The cross-list guard for #907: every resource list, driven through its REAL
 * column builder and the REAL <ListTable>, must agree on the three things that
 * had drifted five ways — a relative timestamp carrying its absolute value in
 * `title`, one status badge, and an `<action>-<singular>-button` test id on
 * every row action.
 *
 * It lives at this level deliberately: a factory unit test calling a cell
 * renderer directly still passes for a column no page ever registered.
 */

const NOW = new Date('2024-06-01T12:00:00Z')
const THREE_HOURS_AGO = '2024-06-01T09:00:00Z'

const common = {
  navigate: vi.fn(),
  onDelete: vi.fn(),
  canDelete: () => true,
}

interface ListCase {
  list: string
  singular: string
  columns: ColumnDef<{ id: string }>[]
  row: Record<string, unknown>
  statusText: string
  /**
   * The solid fill `StatusBadge` gives that status's descriptor tone. This is
   * the discriminator, NOT `border-transparent` — three of the four plain
   * `Badge` variants carry that too (`ui/badge.tsx`), so asserting it would
   * pass for a hand-rolled badge.
   */
  statusToneClass: string
  /** Every action the list offers, in the order the buttons render. */
  actions: readonly string[]
}

function listCases(): ListCase[] {
  const base = {
    id: 'r1',
    project_id: 'p1',
    slug: 'runbook',
    updated_at: THREE_HOURS_AGO,
  }
  const asColumns = (columns: unknown) => columns as ColumnDef<{ id: string }>[]
  return [
    {
      list: 'prompts',
      singular: 'prompt',
      columns: asColumns(buildPromptsColumns(common)),
      row: { ...base, name: 'Runbook', status: 'published', labels: ['a'] },
      statusText: 'Published',
      statusToneClass: 'bg-success',
      actions: ['view', 'edit', 'delete'],
    },
    {
      list: 'artifacts',
      singular: 'artifact',
      columns: asColumns(buildArtifactsColumns(common)),
      row: { ...base, title: 'Runbook', status: 'active', type: 'general' },
      statusText: 'Active',
      statusToneClass: 'bg-success',
      actions: ['view', 'edit', 'delete'],
    },
    {
      list: 'blueprints',
      singular: 'blueprint',
      columns: asColumns(buildBlueprintsColumns(common)),
      row: { ...base, title: 'Runbook', status: 'expired', type: 'cursor' },
      statusText: 'Expired',
      statusToneClass: 'bg-muted',
      actions: ['view', 'edit', 'delete'],
    },
    {
      list: 'memories',
      singular: 'memory',
      columns: asColumns(
        buildMemoriesColumns({ ...common, includeTags: true, projects: [] })
      ),
      row: { ...base, text: 'Remember this', status: 'archived', metadata: {} },
      statusText: 'Archived',
      statusToneClass: 'bg-muted',
      actions: ['view', 'edit', 'delete'],
    },
    {
      list: 'agents',
      singular: 'agent',
      columns: asColumns(buildAgentsColumns(common)),
      row: {
        ...base,
        name: 'Scout',
        status: 'active',
        total_runs: 3,
        success_rate: 91,
        last_run: THREE_HOURS_AGO,
      },
      statusText: 'Active',
      statusToneClass: 'bg-success',
      actions: ['chat', 'edit', 'delete'],
    },
  ]
}

function renderList(testCase: ListCase) {
  return render(
    <ListTable
      rows={[testCase.row as { id: string }]}
      columns={testCase.columns}
    />
  )
}

describe.each(listCases())('$list list columns', testCase => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders its timestamp relative, with the absolute time in `title`', () => {
    renderList(testCase)
    const label = screen.getByText('3h ago')
    expect(label).toHaveAttribute('title', expect.stringContaining('2024'))
    expect(label.getAttribute('title')).not.toBe(label.textContent)
  })

  it('renders its status through the one status badge', () => {
    renderList(testCase)
    // The tone comes off the descriptor, so a list that stopped calling
    // `statusColumn` and hand-rolled a <Badge> again cannot land on it: two of
    // the five cases are `bg-muted`, which no default badge renders.
    expect(screen.getByText(testCase.statusText)).toHaveClass(
      testCase.statusToneClass
    )
  })

  it('gives every row action a test id', () => {
    renderList(testCase)
    const ids = testCase.actions.map(
      action => `${action}-${testCase.singular}-button`
    )
    for (const id of ids) {
      expect(screen.getByTestId(id)).toBeInTheDocument()
    }
  })
})
