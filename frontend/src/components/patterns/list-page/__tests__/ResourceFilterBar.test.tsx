import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { ResourceKindKey } from '@/components/patterns/resource'
import { getResourceDescriptor } from '@/components/patterns/resource'

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ id: 't1', slug: 'work_reports', name: 'Work reports' }],
    isLoading: false,
    reload: vi.fn(),
  }),
}))

vi.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team', permissions: [] }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

// The metadata control has its own suite; here only its presence matters.
vi.mock('@/components/metadata/MetadataFilterField', () => ({
  MetadataFilterField: ({ ariaLabel }: { ariaLabel?: string }) => (
    <button type="button" aria-label={ariaLabel}>
      Metadata
    </button>
  ),
}))

const getPromptLabels = vi.fn()
vi.mock('@/services/promptService', () => ({
  promptService: {
    getPromptLabels: (teamId: string) => getPromptLabels(teamId) as unknown,
  },
}))

// Radix Select and Popover both need layout APIs jsdom lacks.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

import type { UseResourceListFiltersResult } from '@/hooks/useResourceListFilters'

import { ResourceFilterBar } from '../ResourceFilterBar'

interface Overrides {
  kind?: ResourceKindKey
  values?: Record<string, string>
  hasActiveFilters?: boolean
  extras?: React.ReactNode
}

const setFilters = vi.fn()
const setSearchInput = vi.fn()
const setMetadata = vi.fn()
const handleClear = vi.fn()

/** A stand-in for what `useResourceListFilters` hands the bar. */
function filtersState(
  values: Record<string, string>,
  hasActiveFilters: boolean
): UseResourceListFiltersResult {
  return {
    filters: {
      page: '1',
      search: '',
      metadata: '',
      sort_order: 'desc',
      ...values,
    },
    setFilters,
    searchInput: '',
    setSearchInput,
    page: 1,
    setPage: vi.fn(),
    sortOrder: 'desc',
    metadata: {},
    metadataParam: undefined,
    setMetadata,
    hasActiveFilters,
    handleClear,
  }
}

function renderBar({
  kind = 'artifact',
  values = {},
  hasActiveFilters = false,
  extras,
}: Overrides = {}) {
  return render(
    <ResourceFilterBar
      descriptor={getResourceDescriptor(kind)}
      filters={filtersState(values, hasActiveFilters)}
      extras={extras}
    />
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  getPromptLabels.mockResolvedValue(['api', 'review'])
})

describe('ResourceFilterBar', () => {
  it.each([
    ['artifact', 5],
    ['blueprint', 5],
    ['memory', 4],
    ['prompt', 4],
  ] as [ResourceKindKey, number][])(
    'renders one control per %s filter spec',
    (kind, expected) => {
      renderBar({ kind })
      const specs = getResourceDescriptor(kind).list?.filters ?? []
      expect(specs).toHaveLength(expected)
      for (const spec of specs) {
        expect(screen.getByLabelText(spec.label)).toBeInTheDocument()
      }
    }
  )

  it('reports search typing without touching the committed filters', async () => {
    const user = userEvent.setup()
    renderBar()
    await user.type(screen.getByLabelText('Search artifacts'), 'x')
    expect(setSearchInput).toHaveBeenCalledWith('x')
    expect(setFilters).not.toHaveBeenCalled()
  })

  it('emits the filter key and the raw value when a status is picked', async () => {
    const user = userEvent.setup()
    renderBar({ kind: 'blueprint' })
    await user.click(screen.getByLabelText('Filter by status'))
    await user.click(await screen.findByRole('option', { name: 'Expired' }))
    expect(setFilters).toHaveBeenCalledWith({ status: 'expired' })
  })

  it('offers exactly the status values the descriptor declares', async () => {
    const user = userEvent.setup()
    renderBar({ kind: 'blueprint' })
    await user.click(screen.getByLabelText('Filter by status'))
    const options = (await screen.findAllByRole('option')).map(
      option => option.textContent
    )
    expect(options).toEqual(['All statuses', 'Active', 'Expired'])
  })

  it('reads artifact type options from the team catalog, not the field', async () => {
    const user = userEvent.setup()
    renderBar()
    await user.click(screen.getByLabelText('Filter by type'))
    expect(
      await screen.findByRole('option', { name: 'Work reports' })
    ).toBeInTheDocument()
    // `static_contexts` is a label on the field but not a registered type.
    expect(
      screen.queryByRole('option', { name: 'Static contexts' })
    ).not.toBeInTheDocument()
  })

  it('shows the committed value rather than the placeholder', () => {
    renderBar({ kind: 'memory', values: { status: 'archived' } })
    expect(screen.getByLabelText('Filter by status')).toHaveTextContent(
      'Archived'
    )
  })

  it('clears a select back to the shared "all" sentinel', async () => {
    const user = userEvent.setup()
    renderBar({ kind: 'memory', values: { status: 'archived' } })
    await user.click(screen.getByLabelText('Filter by status'))
    await user.click(
      await screen.findByRole('option', { name: 'All statuses' })
    )
    expect(setFilters).toHaveBeenCalledWith({ status: 'all' })
  })

  it('clears freshness back to the sentinel too', async () => {
    const user = userEvent.setup()
    renderBar({ values: { freshness: 'stale' } })
    await user.click(screen.getByLabelText('Filter artifacts by freshness'))
    await user.click(
      await screen.findByRole('option', { name: 'All freshness' })
    )
    expect(setFilters).toHaveBeenCalledWith({ freshness: 'all' })
  })

  it('loads the label catalog only when the taxonomy popover opens', async () => {
    const user = userEvent.setup()
    renderBar({ kind: 'prompt' })
    expect(getPromptLabels).not.toHaveBeenCalled()

    await user.click(screen.getByLabelText('Filter by labels'))
    await waitFor(() => {
      expect(getPromptLabels).toHaveBeenCalledWith('team-1')
    })
    expect(
      await screen.findByRole('option', { name: /review/ })
    ).toBeInTheDocument()
  })

  it('serializes picked labels as the comma-separated list the API takes', async () => {
    const user = userEvent.setup()
    renderBar({ kind: 'prompt', values: { labels: 'api' } })
    await user.click(screen.getByLabelText('Filter by labels'))
    await user.click(await screen.findByRole('option', { name: /review/ }))
    expect(setFilters).toHaveBeenCalledWith({ labels: 'api,review' })
  })

  it('renders no metadata control for a kind whose descriptor declares none', () => {
    // The prompts list endpoint has no `metadata` parameter, so the prompt
    // descriptor declares no such filter and the bar must not invent one.
    renderBar({ kind: 'prompt' })
    expect(screen.queryByLabelText(/by metadata/)).not.toBeInTheDocument()
    expect(screen.getByLabelText('Search prompts')).toBeInTheDocument()
  })

  it('renders resource-specific extras alongside the generated controls', () => {
    renderBar({ kind: 'prompt', extras: <span>Shared control</span> })
    expect(screen.getByText('Shared control')).toBeInTheDocument()
  })

  it('offers Clear only while a filter is applied', async () => {
    const user = userEvent.setup()
    const { rerender } = renderBar()
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()

    rerender(
      <ResourceFilterBar
        descriptor={getResourceDescriptor('artifact')}
        filters={filtersState({ status: 'draft' }, true)}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(handleClear).toHaveBeenCalledTimes(1)
  })

  it('renders every control at the one shared width', () => {
    const { container } = renderBar({ kind: 'memory' })
    const triggers = container.querySelectorAll('[role="combobox"]')
    expect(triggers.length).toBeGreaterThan(0)
    for (const trigger of triggers) {
      expect(trigger).toHaveClass('w-[150px]')
    }
  })

  it('keeps the test ids the existing page suites drive', () => {
    const { container } = renderBar()
    expect(
      within(container).getByTestId('artifact-freshness-filter')
    ).toBeInTheDocument()
  })
})
