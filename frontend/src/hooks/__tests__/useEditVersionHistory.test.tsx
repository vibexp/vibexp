import { renderHook, waitFor } from '@testing-library/react'

const services = vi.hoisted(() => {
  const team: { current: { id: string } | null } = {
    current: { id: 'team-1' },
  }
  return {
    artifact: vi.fn(),
    blueprint: vi.fn(),
    memory: vi.fn(),
    prompt: vi.fn(),
    team,
  }
})

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: services.team.current }),
}))
vi.mock('@/services/artifactService', () => ({
  artifactService: { getArtifactVersions: services.artifact },
}))
vi.mock('@/services/blueprintService', () => ({
  blueprintService: { getBlueprintVersions: services.blueprint },
}))
vi.mock('@/services/memoryService', () => ({
  memoryService: { getMemoryVersions: services.memory },
}))
vi.mock('@/services/promptService', () => ({
  promptService: { getPromptVersions: services.prompt },
}))

import { useEditVersionHistory } from '@/hooks/useEditVersionHistory'

const LIST = {
  versions: [{ version_number: 1 }, { version_number: 2 }],
  total: 2,
}

describe('useEditVersionHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    services.team.current = { id: 'team-1' }
    for (const load of [
      services.artifact,
      services.blueprint,
      services.memory,
      services.prompt,
    ]) {
      load.mockResolvedValue(LIST)
    }
  })

  it.each([
    [
      { type: 'artifact', id: 'a1', projectId: 'p 1', slug: 'a-b' },
      services.artifact,
      ['team-1', 'p 1', 'a-b'],
      '/artifacts/p%201/a-b/versions',
    ],
    [
      { type: 'blueprint', id: 'b1', projectId: 'p1', slug: 'rules' },
      services.blueprint,
      ['team-1', 'p1', 'rules'],
      '/blueprints/p1/rules/versions',
    ],
    [
      { type: 'prompt', id: 'pr1', slug: 'sum+up' },
      services.prompt,
      ['team-1', 'sum+up'],
      '/prompts/sum%2Bup/versions',
    ],
    [
      { type: 'memory', id: 'm1' },
      services.memory,
      ['team-1', 'm1'],
      '/memories/m1/versions',
    ],
  ] as const)(
    'loads the %o history and links to its route',
    async (resource, load, args, to) => {
      const { result } = renderHook(() =>
        useEditVersionHistory({
          ...resource,
          updatedAt: '2026-09-01T00:00:00Z',
        })
      )
      await waitFor(() => {
        expect(result.current).toBeDefined()
      })
      expect(load).toHaveBeenCalledWith(...args)
      expect(result.current).toMatchObject({
        to,
        count: 2,
        editedAt: '2026-09-01T00:00:00Z',
      })
    }
  )

  it('loads nothing before the resource resolves', () => {
    const { result } = renderHook(() => useEditVersionHistory(null))
    expect(result.current).toBeUndefined()
    expect(services.artifact).not.toHaveBeenCalled()
  })

  it('loads nothing for an incomplete address', () => {
    const { result } = renderHook(() =>
      useEditVersionHistory({ type: 'artifact', id: 'a1', slug: 'x' })
    )
    expect(result.current).toBeUndefined()
    expect(services.artifact).not.toHaveBeenCalled()
  })

  it('loads nothing without a team', () => {
    services.team.current = null
    const { result } = renderHook(() =>
      useEditVersionHistory({ type: 'memory', id: 'm1' })
    )
    expect(result.current).toBeUndefined()
    expect(services.memory).not.toHaveBeenCalled()
  })
})
