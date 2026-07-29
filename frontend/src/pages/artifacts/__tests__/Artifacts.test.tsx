import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mock } from 'vitest'

import type { Project } from '@/services/projectService'

// Mock Radix Select — it can loop in JSDOM (same approach as Feeds.test.tsx)
vi.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode
    value: string
  }) => <div data-value={value}>{children}</div>,
}))

vi.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifacts: vi.fn(),
    deleteArtifact: vi.fn(),
  },
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({ types: [], loading: false }),
}))

// Only the catalog client is mocked; parseMetadataFilter/serializeMetadataFilter
// stay real, since the URL round-trip is exactly what these tests assert.
vi.mock('@/services/metadataService', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/metadataService')
  >('@/services/metadataService')
  return {
    ...actual,
    metadataService: { listKeys: vi.fn(), listValues: vi.fn() },
  }
})

// MetadataFilter is a Radix Popover + cmdk, which the Select mock above does
// not cover; these are the layout APIs jsdom lacks.
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

// Mock TeamContext — stable references
// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

// Mock ProjectContext — mutable so tests choose the global selection
const projectContextValue: {
  currentProject: Project | null
  setCurrentProject: Mock
  isLoading: boolean
} = {
  currentProject: null,
  setCurrentProject: vi.fn(),
  isLoading: false,
}
vi.mock('@/contexts/ProjectContext', () => ({
  useProject: () => projectContextValue,
}))

vi.mock('@/hooks', () => {
  const showSuccess = vi.fn()
  const showError = vi.fn()
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

import React from 'react'

import { artifactService } from '@/services/artifactService'
import { metadataService } from '@/services/metadataService'

import { Artifacts } from '../Artifacts'

const emptyResponse = {
  artifacts: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const alpha: Project = {
  id: 'p1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

/** The filter object of the most recent getArtifacts call. */
const lastQuery = () => {
  const { calls } = (artifactService.getArtifacts as Mock).mock
  return calls[calls.length - 1][1] as Record<string, unknown>
}

function artifactsTree(initialEntry: string) {
  return (
    <MemoryRouter initialEntries={[initialEntry]}>
      <Artifacts />
      <LocationProbe />
    </MemoryRouter>
  )
}

function renderArtifacts(initialEntry = '/artifacts') {
  currentSearch = ''
  return render(artifactsTree(initialEntry))
}

describe('Artifacts page — global project filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(artifactService.getArtifacts as Mock).mockResolvedValue(emptyResponse)
    ;(metadataService.listKeys as Mock).mockResolvedValue({
      keys: ['env', 'team'],
      truncated: false,
    })
    ;(metadataService.listValues as Mock).mockResolvedValue({
      values: ['prod', 'staging'],
      truncated: false,
    })
  })

  it('fetches without project_id under "All projects"', async () => {
    renderArtifacts()

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: undefined })
      )
    })
  })

  it('fetches scoped to the globally selected project', async () => {
    projectContextValue.currentProject = alpha
    renderArtifacts()

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: 'p1' })
      )
    })
  })

  it('does not fetch while the persisted project selection is restoring', async () => {
    projectContextValue.isLoading = true
    renderArtifacts()

    // Flush pending effects/microtasks, then assert no fetch happened
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(artifactService.getArtifacts).not.toHaveBeenCalled()
  })

  it('a selected project alone is NOT a page filter', async () => {
    // The project comes from the global header selector, not this page's filter
    // bar, so counting it would promise a `Clear filters` that cannot clear it
    // (#523, matching #522).
    projectContextValue.currentProject = alpha
    renderArtifacts()

    await waitFor(() => {
      expect(screen.getByText('No artifacts yet')).toBeInTheDocument()
    })
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
  })

  it('a restoring persisted project does not clobber a shared link’s page', async () => {
    projectContextValue.isLoading = true
    projectContextValue.currentProject = null

    const { rerender } = renderArtifacts('/artifacts?page=3')
    expect(artifactService.getArtifacts).not.toHaveBeenCalled()

    projectContextValue.isLoading = false
    projectContextValue.currentProject = alpha
    rerender(artifactsTree('/artifacts?page=3'))

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    expect(lastQuery().page).toBe(3)
    expect(lastQuery().project_id).toBe('p1')
  })
})

describe('Artifacts page — URL-synced filters (#523)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(artifactService.getArtifacts as Mock).mockResolvedValue(emptyResponse)
    ;(metadataService.listKeys as Mock).mockResolvedValue({
      keys: ['env', 'team'],
      truncated: false,
    })
    ;(metadataService.listValues as Mock).mockResolvedValue({
      values: ['prod', 'staging'],
      truncated: false,
    })
  })

  it('keeps defaults out of the URL and sends no filter params', async () => {
    renderArtifacts()

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    expect(lastQuery()).toEqual(
      expect.objectContaining({
        page: 1,
        limit: 20,
        search: undefined,
        type: undefined,
        status: undefined,
        metadata: undefined,
        sort_order: 'desc',
      })
    )
    expect(currentSearch).toBe('')
  })

  it('rehydrates search, status and metadata from the URL on mount', async () => {
    renderArtifacts(
      '/artifacts?search=spec&status=draft&metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D&page=2'
    )

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    const query = lastQuery()
    expect(query.search).toBe('spec')
    expect(query.status).toBe('draft')
    // The param goes on the wire as the raw JSON object, encoded exactly once.
    expect(query.metadata).toBe('{"env":["prod"]}')
    expect(query.page).toBe(2)
    expect(
      screen.getByRole('textbox', { name: 'Search artifacts' })
    ).toHaveValue('spec')
    expect(screen.getByTestId('metadata-chip-env')).toHaveTextContent(
      'env: prod'
    )
  })

  it('ignores a malformed metadata param instead of sending garbage', async () => {
    renderArtifacts('/artifacts?metadata=not-json')

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    expect(lastQuery().metadata).toBeUndefined()
    expect(screen.queryByTestId('metadata-chip-env')).not.toBeInTheDocument()
    expect(await screen.findByText('No artifacts yet')).toBeInTheDocument()
  })

  it('ignores an empty type param rather than forwarding an empty string', async () => {
    // `type` is an open string, so unlike `status` there is no enum coercion to
    // absorb a junk value.
    renderArtifacts('/artifacts?type=')

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    expect(lastQuery().type).toBeUndefined()
  })

  it('ignores a status outside the enum rather than forwarding it', async () => {
    renderArtifacts('/artifacts?status=nonsense')

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    expect(lastQuery().status).toBeUndefined()
  })

  it('committing a metadata chip sends the JSON param and resets to page 1', async () => {
    renderArtifacts('/artifacts?page=4')
    await waitFor(() => {
      expect(lastQuery().page).toBe(4)
    })

    const user = userEvent.setup()
    await user.click(
      screen.getByRole('combobox', { name: 'Filter artifacts by metadata' })
    )
    await user.click(await screen.findByText('env'))
    await user.click(await screen.findByText('prod'))
    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    await waitFor(() => {
      expect(lastQuery().metadata).toBe('{"env":["prod"]}')
    })
    expect(lastQuery().page).toBe(1)
    expect(currentSearch).toContain('metadata=')
  })

  it('removing the chip drops the param entirely', async () => {
    renderArtifacts('/artifacts?metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D')
    await waitFor(() => {
      expect(lastQuery().metadata).toBe('{"env":["prod"]}')
    })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Remove env filter' }))

    await waitFor(() => {
      expect(lastQuery().metadata).toBeUndefined()
    })
    expect(currentSearch).not.toContain('metadata')
  })

  it('debounces the search box into a single request', async () => {
    renderArtifacts()
    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalled()
    })
    const before = (artifactService.getArtifacts as Mock).mock.calls.length

    const user = userEvent.setup()
    await user.type(
      screen.getByRole('textbox', { name: 'Search artifacts' }),
      'api'
    )

    await waitFor(
      () => {
        expect(lastQuery().search).toBe('api')
      },
      { timeout: 2000 }
    )
    expect((artifactService.getArtifacts as Mock).mock.calls.length).toBe(
      before + 1
    )
    expect(currentSearch).toContain('search=api')
  })

  it('Clear filters empties the URL, the search box and the chips', async () => {
    renderArtifacts(
      '/artifacts?search=nope&status=draft&metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D'
    )
    await screen.findByText('No artifacts match your filters')

    const user = userEvent.setup()
    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await user.click(clear)

    await waitFor(() => {
      expect(lastQuery().search).toBeUndefined()
    })
    expect(lastQuery().status).toBeUndefined()
    expect(lastQuery().metadata).toBeUndefined()
    expect(currentSearch).toBe('')
    expect(
      screen.getByRole('textbox', { name: 'Search artifacts' })
    ).toHaveValue('')
    expect(screen.queryByTestId('metadata-chip-env')).not.toBeInTheDocument()
  })
})
