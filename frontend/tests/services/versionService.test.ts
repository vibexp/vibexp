import type { Blueprint, Memory, Prompt } from '../../src/types'

const mockBlueprintService = {
  getBlueprint: jest.fn(),
  getBlueprintVersions: jest.fn(),
  restoreBlueprintVersion: jest.fn(),
}

const mockMemoryService = {
  getMemory: jest.fn(),
  getMemoryVersions: jest.fn(),
  restoreMemoryVersion: jest.fn(),
}

const mockPromptService = {
  getPrompt: jest.fn(),
  getPromptVersions: jest.fn(),
  restorePromptVersion: jest.fn(),
}

jest.mock('../../src/services/blueprintService', () => ({
  blueprintService: mockBlueprintService,
}))

jest.mock('../../src/services/memoryService', () => ({
  memoryService: mockMemoryService,
}))

jest.mock('../../src/services/promptService', () => ({
  promptService: mockPromptService,
}))

// artifactService is imported by versionService but not exercised here.
jest.mock('../../src/services/artifactService', () => ({
  artifactService: {},
}))

import {
  createBlueprintVersionSource,
  createMemoryVersionSource,
  createPromptVersionSource,
} from '../../src/services/versionService'

describe('createBlueprintVersionSource', () => {
  const teamId = 'team-123'
  const projectId = 'proj-1'
  const slug = 'my-blueprint'
  const backHref = `/blueprints/${projectId}/${slug}`

  const blueprint: Blueprint = {
    id: 'bp-1',
    project_id: projectId,
    slug,
    user_id: 'user-123',
    title: 'My Blueprint',
    type: 'general',
    status: 'active',
    description: '',
    content: 'live content',
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  }

  const versions = [
    {
      id: 'ver-1',
      team_id: teamId,
      resource_type: 'blueprint',
      resource_id: 'bp-1',
      version_number: 1,
      content: 'old content',
      change_summary: null,
      actor_type: 'human' as const,
      created_by: 'user-123',
      author: null,
      created_at: '2026-01-01T00:00:00Z',
    },
  ]

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('describes the blueprint resource', () => {
    const source = createBlueprintVersionSource({
      teamId,
      projectId,
      slug,
      backHref,
    })
    expect(source.resourceType).toBe('blueprint')
    expect(source.resourceLabel).toBe('blueprint')
    expect(source.backHref).toBe(backHref)
  })

  it('load() fetches the blueprint + versions and maps the timeline data', async () => {
    mockBlueprintService.getBlueprint.mockResolvedValue(blueprint)
    mockBlueprintService.getBlueprintVersions.mockResolvedValue({ versions })

    const source = createBlueprintVersionSource({
      teamId,
      projectId,
      slug,
      backHref,
    })
    const data = await source.load()

    expect(mockBlueprintService.getBlueprint).toHaveBeenCalledWith(
      teamId,
      projectId,
      slug
    )
    expect(mockBlueprintService.getBlueprintVersions).toHaveBeenCalledWith(
      teamId,
      projectId,
      slug
    )
    expect(data).toEqual({
      currentContent: 'live content',
      currentUpdatedAt: '2026-01-02T00:00:00Z',
      resourceName: 'My Blueprint',
      versions,
    })
  })

  it('restore() delegates to restoreBlueprintVersion', async () => {
    mockBlueprintService.restoreBlueprintVersion.mockResolvedValue(blueprint)

    const source = createBlueprintVersionSource({
      teamId,
      projectId,
      slug,
      backHref,
    })
    await source.restore(2)

    expect(mockBlueprintService.restoreBlueprintVersion).toHaveBeenCalledWith(
      teamId,
      projectId,
      slug,
      2
    )
  })
})

describe('createMemoryVersionSource', () => {
  const teamId = 'team-123'
  const id = 'mem-1'
  const backHref = `/memories/${id}`

  const memory: Memory = {
    id,
    user_id: 'user-123',
    team_id: teamId,
    project_id: 'proj-1',
    text: 'live text',
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
  }

  const versions = [
    {
      id: 'ver-1',
      team_id: teamId,
      resource_type: 'memory',
      resource_id: id,
      version_number: 1,
      content: 'old text',
      change_summary: null,
      actor_type: 'human' as const,
      created_by: 'user-123',
      author: null,
      created_at: '2026-01-01T00:00:00Z',
    },
  ]

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('describes the memory resource', () => {
    const source = createMemoryVersionSource({ teamId, id, backHref })
    expect(source.resourceType).toBe('memory')
    expect(source.resourceLabel).toBe('memory')
    expect(source.backHref).toBe(backHref)
  })

  it('load() fetches the memory + versions and maps the timeline data (id-derived name)', async () => {
    mockMemoryService.getMemory.mockResolvedValue(memory)
    mockMemoryService.getMemoryVersions.mockResolvedValue({ versions })

    const source = createMemoryVersionSource({ teamId, id, backHref })
    const data = await source.load()

    expect(mockMemoryService.getMemory).toHaveBeenCalledWith(teamId, id)
    expect(mockMemoryService.getMemoryVersions).toHaveBeenCalledWith(teamId, id)
    expect(data).toEqual({
      currentContent: 'live text',
      currentUpdatedAt: '2026-01-02T00:00:00Z',
      resourceName: `Memory #${id}`,
      versions,
    })
  })

  it('restore() delegates to restoreMemoryVersion', async () => {
    mockMemoryService.restoreMemoryVersion.mockResolvedValue(memory)

    const source = createMemoryVersionSource({ teamId, id, backHref })
    await source.restore(2)

    expect(mockMemoryService.restoreMemoryVersion).toHaveBeenCalledWith(
      teamId,
      id,
      2
    )
  })
})

describe('createPromptVersionSource', () => {
  const teamId = 'team-123'
  const slug = 'my-prompt'
  const backHref = `/prompts/${slug}`

  const prompt: Prompt = {
    id: 'prompt-1',
    name: 'My Prompt',
    slug,
    description: '',
    body: 'live {{name}} @intro',
    user_id: 'user-123',
    project_id: 'proj-1',
    status: 'published',
    mcp_expose: false,
    is_shared: false,
    labels: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 3,
  }

  const versions = [
    {
      id: 'ver-1',
      team_id: teamId,
      resource_type: 'prompt',
      resource_id: 'prompt-1',
      version_number: 1,
      content: 'old {{name}}',
      change_summary: null,
      actor_type: 'human' as const,
      created_by: 'user-123',
      author: null,
      created_at: '2026-01-01T00:00:00Z',
    },
  ]

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('describes the prompt resource', () => {
    const source = createPromptVersionSource({ teamId, slug, backHref })
    expect(source.resourceType).toBe('prompt')
    expect(source.resourceLabel).toBe('prompt')
    expect(source.backHref).toBe(backHref)
  })

  it('load() fetches the prompt + versions and maps the raw body template', async () => {
    mockPromptService.getPrompt.mockResolvedValue(prompt)
    mockPromptService.getPromptVersions.mockResolvedValue({ versions })

    const source = createPromptVersionSource({ teamId, slug, backHref })
    const data = await source.load()

    expect(mockPromptService.getPrompt).toHaveBeenCalledWith(teamId, slug)
    expect(mockPromptService.getPromptVersions).toHaveBeenCalledWith(
      teamId,
      slug
    )
    expect(data).toEqual({
      // The raw body template is versioned, not rendered output.
      currentContent: 'live {{name}} @intro',
      currentUpdatedAt: '2026-01-02T00:00:00Z',
      resourceName: 'My Prompt',
      versions,
    })
  })

  it('restore() delegates to restorePromptVersion', async () => {
    mockPromptService.restorePromptVersion.mockResolvedValue(prompt)

    const source = createPromptVersionSource({ teamId, slug, backHref })
    await source.restore(2)

    expect(mockPromptService.restorePromptVersion).toHaveBeenCalledWith(
      teamId,
      slug,
      2
    )
  })
})
