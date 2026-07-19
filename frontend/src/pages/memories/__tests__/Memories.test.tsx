/**
 * Unit tests for the Memories list page's supporting modules
 * (memoriesColumns helpers) in isolation.
 *
 * The Memories.tsx page component itself is rendered and tested in
 * MemoriesPage.test.tsx, which stubs the Radix UI Select (it can loop in
 * Jest/JSDOM) with a lightweight interactive mock.
 */

import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

import { buildMemoriesColumns, extractTags } from '../memoriesColumns'

const makeMemory = (overrides: Partial<Memory> = {}): Memory => ({
  id: 'mem-1',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-alpha',
  text: 'Sample memory',
  status: 'active',
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
  github_connected: false,
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
      canDelete: () => true,
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
      canDelete: () => true,
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
      canDelete: () => true,
      includeTags: false,
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).not.toContain('project')
  })

  it('includes tags column when includeTags is true', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      canDelete: () => true,
      includeTags: true,
      projects: [],
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).toContain('tags')
  })

  it('always includes text, status, updated_at, and actions columns', () => {
    const columns = buildMemoriesColumns({
      navigate: navigateMock,
      onDelete: onDeleteMock,
      canDelete: () => true,
      includeTags: false,
    })

    const ids = columns.map(c => ('accessorKey' in c ? c.accessorKey : c.id))
    expect(ids).toContain('text')
    expect(ids).toContain('status')
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
