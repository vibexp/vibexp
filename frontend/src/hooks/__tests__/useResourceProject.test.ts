import { renderHook, waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

import { projectService } from '@/services/projectService'

import { useResourceProject } from '../useResourceProject'

vi.mock('@/services/projectService', () => ({
  projectService: { getProjects: vi.fn() },
}))

const PROJECT = { id: 'p1', name: 'Design System', slug: 'design-system' }

function mockProjects(projects: unknown[]) {
  ;(projectService.getProjects as Mock).mockResolvedValue({ projects })
}

describe('useResourceProject', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('resolves the project matching the resource project id', async () => {
    mockProjects([{ id: 'other', name: 'Other', slug: 'other' }, PROJECT])

    const { result } = renderHook(() => useResourceProject('team-1', 'p1'))

    await waitFor(() => {
      expect(result.current).toEqual(PROJECT)
    })
    expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {
      limit: 100,
    })
  })

  it('fetches nothing without a team or a project id', () => {
    const { result } = renderHook(() => useResourceProject(undefined, 'p1'))
    expect(result.current).toBeNull()

    const second = renderHook(() => useResourceProject('team-1', undefined))
    expect(second.result.current).toBeNull()
    expect(projectService.getProjects).not.toHaveBeenCalled()
  })

  it('stays null when the id is not in the returned page', async () => {
    mockProjects([{ id: 'other', name: 'Other', slug: 'other' }])

    const { result } = renderHook(() => useResourceProject('team-1', 'p1'))

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })
    expect(result.current).toBeNull()
  })

  it('swallows a failed lookup — the row is supplemental, not a page error', async () => {
    ;(projectService.getProjects as Mock).mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useResourceProject('team-1', 'p1'))

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalled()
    })
    expect(result.current).toBeNull()
  })

  it('ignores a slower earlier response after the project id changes', async () => {
    const slow = { id: 'p1', name: 'Stale', slug: 'stale' }
    const fast = { id: 'p2', name: 'Fresh', slug: 'fresh' }
    let releaseFirst: (v: unknown) => void = () => undefined
    ;(projectService.getProjects as Mock)
      .mockImplementationOnce(
        () =>
          new Promise(resolve => {
            releaseFirst = resolve
          })
      )
      .mockResolvedValueOnce({ projects: [fast] })

    const { result, rerender } = renderHook(
      ({ id }: { id: string }) => useResourceProject('team-1', id),
      { initialProps: { id: 'p1' } }
    )
    rerender({ id: 'p2' })

    await waitFor(() => {
      expect(result.current).toEqual(fast)
    })

    releaseFirst({ projects: [slow] })
    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalledTimes(2)
    })
    expect(result.current).toEqual(fast)
  })
})
