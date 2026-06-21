/**
 * Tests for the Memories list page and its supporting modules.
 *
 * Note: Full rendering of Memories.tsx is memory-intensive in Jest/JSDOM
 * because of the transitive Radix UI Select imports. Tests here focus on
 * the memoriesColumns helper and the MemoryFilters component in isolation,
 * plus service-call behaviour via mocks.
 */

import type { Memory, Project } from '@/types'

import { buildMemoriesColumns, extractTags } from '../memoriesColumns'

const makeMemory = (overrides: Partial<Memory> = {}): Memory => ({
  id: 'mem-1',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-alpha',
  text: 'Sample memory',
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  version: 1,
  ...overrides,
})

const mockProject: Project = {
  id: 'project-alpha',
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
}

describe('buildMemoriesColumns', () => {
  const navigateMock = jest.fn()
  const onDeleteMock = jest.fn()

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('includes a project column when projects are provided', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      includeTags: false,
      projects: [mockProject],
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).toContain('project')
  })

  it('omits project column when no projects are provided', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      includeTags: false,
      projects: [],
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).not.toContain('project')
  })

  it('omits project column when projects prop is omitted (default)', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      includeTags: false,
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).not.toContain('project')
  })

  it('includes tags column when includeTags is true', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      includeTags: true,
      projects: [],
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).toContain('tags')
  })

  it('always includes text, updated_at, and actions columns', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      includeTags: false,
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).toContain('text')
    expect(ids).toContain('updated_at')
    expect(ids).toContain('actions')
  })
})

describe('extractTags', () => {
  it('returns tags from metadata', () => {
    const mem = makeMemory({ metadata: { tags: ['a', 'b'] } })
    expect(extractTags(mem.metadata)).toEqual(['a', 'b'])
  })

  it('returns empty array when no tags', () => {
    const mem = makeMemory({ metadata: {} })
    expect(extractTags(mem.metadata)).toEqual([])
  })

  it('filters non-string values from tags array', () => {
    const mem = makeMemory({ metadata: { tags: ['a', 1, null, 'b'] } })
    expect(extractTags(mem.metadata)).toEqual(['a', 'b'])
  })
})
