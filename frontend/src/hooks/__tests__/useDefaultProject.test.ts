import { renderHook, waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

import type { Project } from '@/services/projectService'

const mockContext = vi.hoisted(() => ({
  currentProject: null as { id: string; name: string } | null,
  isLoading: false,
}))
vi.mock('@/contexts/ProjectContext', () => ({
  useProject: () => ({ ...mockContext, setCurrentProject: vi.fn() }),
}))
vi.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Team' }
  return { useTeam: () => ({ currentTeam, isLoading: false }) }
})
vi.mock('@/services/projectService', () => ({
  projectService: { getProjects: vi.fn() },
}))

import { projectService } from '@/services/projectService'

import { pickDefaultProject, useDefaultProject } from '../useDefaultProject'

const project = (id: string) => ({ id, name: `Project ${id}` }) as Project

describe('pickDefaultProject', () => {
  it('prefers the project chosen in the header filter', () => {
    expect(pickDefaultProject(project('f'), [project('a')])?.id).toBe('f')
  })

  it("falls back to the workspace's only project", () => {
    expect(pickDefaultProject(null, [project('a')])?.id).toBe('a')
  })

  it('picks nothing when there are several, or none', () => {
    expect(pickDefaultProject(null, [project('a'), project('b')])).toBeNull()
    expect(pickDefaultProject(null, [])).toBeNull()
  })
})

describe('useDefaultProject', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockContext.currentProject = null
    mockContext.isLoading = false
  })

  it('answers from the header filter without fetching', () => {
    mockContext.currentProject = { id: 'f', name: 'Filtered' }
    const { result } = renderHook(() => useDefaultProject())

    expect(result.current).toMatchObject({
      project: { id: 'f' },
      loading: false,
    })
    expect(projectService.getProjects).not.toHaveBeenCalled()
  })

  it('waits while a persisted filter is being restored', () => {
    mockContext.isLoading = true
    const { result } = renderHook(() => useDefaultProject())

    expect(result.current.loading).toBe(true)
    expect(projectService.getProjects).not.toHaveBeenCalled()
  })

  it("resolves to the workspace's only project", async () => {
    ;(projectService.getProjects as Mock).mockResolvedValue({
      projects: [project('a')],
    })
    const { result } = renderHook(() => useDefaultProject())

    expect(result.current.loading).toBe(true)
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.project?.id).toBe('a')
    expect(result.current.hasProjects).toBe(true)
  })

  it('leaves the choice open with several projects', async () => {
    ;(projectService.getProjects as Mock).mockResolvedValue({
      projects: [project('a'), project('b')],
    })
    const { result } = renderHook(() => useDefaultProject())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.project).toBeNull()
    expect(result.current.hasProjects).toBe(true)
  })

  it('reports a workspace with no project', async () => {
    ;(projectService.getProjects as Mock).mockResolvedValue({ projects: [] })
    const { result } = renderHook(() => useDefaultProject())

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.hasProjects).toBe(false)
  })

  it('does nothing when disabled', () => {
    const { result } = renderHook(() => useDefaultProject({ enabled: false }))

    expect(result.current).toMatchObject({ project: null, loading: false })
    expect(projectService.getProjects).not.toHaveBeenCalled()
  })
})
