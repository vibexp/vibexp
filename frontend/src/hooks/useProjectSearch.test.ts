import { act, renderHook, waitFor } from '@testing-library/react'

import { useTeam } from '@/contexts/TeamContext'
import { projectService } from '@/services/projectService'
import type { Project, ProjectListResponse } from '@/types/project'

import { useProjectSearch } from './useProjectSearch'

jest.mock('@/contexts/TeamContext')
jest.mock('@/services/projectService')

const mockedUseTeam = useTeam as jest.MockedFunction<typeof useTeam>
const mockedGetProjects = projectService.getProjects as jest.MockedFunction<
  typeof projectService.getProjects
>

const project = (id: string, name: string): Project => ({
  id,
  user_id: 'user-1',
  team_id: 'team-1',
  name,
  slug: name.toLowerCase().replace(/\s+/g, '-'),
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
})

const listResponse = (projects: Project[]): ProjectListResponse => ({
  projects,
  total_count: projects.length,
  page: 1,
  per_page: 100,
  total_pages: 1,
})

function setTeam(): void {
  mockedUseTeam.mockReturnValue({
    currentTeam: {
      id: 'team-1',
      name: 'Team One',
      slug: 'team-one',
    },
    teams: [],
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
    isLoading: false,
  } as unknown as ReturnType<typeof useTeam>)
}

describe('useProjectSearch', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    setTeam()
    mockedGetProjects.mockResolvedValue(listResponse([project('p1', 'Alpha')]))
  })

  afterEach(() => {
    jest.runOnlyPendingTimers()
    jest.useRealTimers()
  })

  it('loads an initial page (no search param) after the debounce', async () => {
    const { result } = renderHook(() => useProjectSearch())

    expect(mockedGetProjects).not.toHaveBeenCalled()

    act(() => {
      jest.advanceTimersByTime(300)
    })

    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenCalledWith('team-1', {
        limit: 100,
        page: 1,
      })
    })
    await waitFor(() => {
      expect(result.current.projects).toHaveLength(1)
    })
  })

  it('debounces the search request with the typed query', async () => {
    const { result } = renderHook(() => useProjectSearch())

    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenCalledTimes(1)
    })

    mockedGetProjects.mockResolvedValue(listResponse([project('p2', 'Beta')]))

    act(() => {
      result.current.setQuery('Be')
    })
    act(() => {
      result.current.setQuery('Beta')
    })

    // Only one trailing call should fire after the debounce window.
    act(() => {
      jest.advanceTimersByTime(299)
    })
    expect(mockedGetProjects).toHaveBeenCalledTimes(1)

    act(() => {
      jest.advanceTimersByTime(1)
    })

    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
        limit: 100,
        page: 1,
        search: 'Beta',
      })
    })
    expect(mockedGetProjects).toHaveBeenCalledTimes(2)
  })

  it('excludes the configured project id from results', async () => {
    mockedGetProjects.mockResolvedValue(
      listResponse([project('p1', 'Alpha'), project('p2', 'Beta')])
    )

    const { result } = renderHook(() =>
      useProjectSearch({ excludeProjectId: 'p1' })
    )

    act(() => {
      jest.advanceTimersByTime(300)
    })

    await waitFor(() => {
      expect(result.current.projects).toHaveLength(1)
    })
    expect(result.current.projects[0]?.id).toBe('p2')
  })

  it('ignores a stale (out-of-order) response so the newest query wins', async () => {
    type Resolve = (value: ProjectListResponse) => void
    let resolveOlder: Resolve = () => undefined
    let resolveNewer: Resolve = () => undefined
    const olderRequest = new Promise<ProjectListResponse>(res => {
      resolveOlder = res
    })
    const newerRequest = new Promise<ProjectListResponse>(res => {
      resolveNewer = res
    })

    mockedGetProjects
      .mockResolvedValueOnce(listResponse([project('p1', 'Alpha')])) // initial
      .mockReturnValueOnce(olderRequest) // query "A" (resolves last)
      .mockReturnValueOnce(newerRequest) // query "AB" (resolves first)

    const { result } = renderHook(() => useProjectSearch())

    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(result.current.projects).toHaveLength(1)
    })

    // Fire the older request, then the newer one — both now in flight.
    act(() => {
      result.current.setQuery('A')
    })
    act(() => {
      jest.advanceTimersByTime(300)
    })
    act(() => {
      result.current.setQuery('AB')
    })
    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenCalledTimes(3)
    })

    // Newer request resolves first…
    await act(async () => {
      resolveNewer(listResponse([project('p-new', 'Newer')]))
      await Promise.resolve()
    })
    await waitFor(() => {
      expect(result.current.projects[0]?.id).toBe('p-new')
    })

    // …then the older request resolves late and must NOT overwrite it.
    await act(async () => {
      resolveOlder(listResponse([project('p-old', 'Older')]))
      await Promise.resolve()
    })

    expect(result.current.projects).toHaveLength(1)
    expect(result.current.projects[0]?.id).toBe('p-new')
  })

  it('surfaces an error message when the fetch fails', async () => {
    mockedGetProjects.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useProjectSearch())

    act(() => {
      jest.advanceTimersByTime(300)
    })

    await waitFor(() => {
      expect(result.current.error).toBe('Failed to load projects')
    })
    expect(result.current.projects).toEqual([])
  })

  const pagedResponse = (
    projects: Project[],
    page: number,
    totalPages: number
  ): ProjectListResponse => ({
    projects,
    total_count: totalPages,
    page,
    per_page: 1,
    total_pages: totalPages,
  })

  it('exposes hasMore and appends the next page via loadMore', async () => {
    mockedGetProjects.mockResolvedValueOnce(
      pagedResponse([project('p1', 'Alpha')], 1, 2)
    )

    const { result } = renderHook(() => useProjectSearch({ limit: 1 }))

    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(result.current.projects).toHaveLength(1)
    })
    expect(result.current.hasMore).toBe(true)

    mockedGetProjects.mockResolvedValueOnce(
      pagedResponse([project('p2', 'Beta')], 2, 2)
    )

    act(() => {
      result.current.loadMore()
    })

    await waitFor(() => {
      expect(result.current.projects).toHaveLength(2)
    })
    expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
      limit: 1,
      page: 2,
    })
    expect(result.current.projects.map(p => p.id)).toEqual(['p1', 'p2'])
    expect(result.current.hasMore).toBe(false)
  })

  it('loadMore is a no-op when no further pages are available', async () => {
    const { result } = renderHook(() => useProjectSearch())

    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(result.current.projects).toHaveLength(1)
    })
    expect(result.current.hasMore).toBe(false)

    act(() => {
      result.current.loadMore()
    })

    expect(mockedGetProjects).toHaveBeenCalledTimes(1)
  })

  it('resets to page 1 (replacing results) when the query changes', async () => {
    // Drive the response off the search param so the assertion doesn't depend
    // on mock-call ordering across the debounce window.
    mockedGetProjects.mockImplementation((_teamId, filters) =>
      Promise.resolve(
        filters?.search
          ? pagedResponse([project('p2', 'Beta')], 1, 1)
          : pagedResponse([project('p1', 'Alpha')], 1, 2)
      )
    )

    const { result } = renderHook(() => useProjectSearch({ limit: 1 }))

    act(() => {
      jest.advanceTimersByTime(300)
    })
    await waitFor(() => {
      expect(result.current.projects.map(p => p.id)).toEqual(['p1'])
    })
    expect(result.current.hasMore).toBe(true)

    act(() => {
      result.current.setQuery('Be')
    })
    act(() => {
      jest.advanceTimersByTime(300)
    })

    await waitFor(() => {
      expect(result.current.projects.map(p => p.id)).toEqual(['p2'])
    })
    expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
      limit: 1,
      page: 1,
      search: 'Be',
    })
    expect(result.current.hasMore).toBe(false)
  })
})
