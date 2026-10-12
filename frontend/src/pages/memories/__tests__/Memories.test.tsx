/**
 * Unit tests for the Memories list page's supporting modules
 * (memoriesColumns helpers) in isolation.
 *
 * The Memories.tsx page component itself is rendered and tested in
 * MemoriesPage.test.tsx, which stubs the Radix UI Select (it can loop in
 * Jest/JSDOM) with a lightweight interactive mock.
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

import { buildMemoriesColumns } from '../memoriesColumns'
import { extractTags } from '../memoryRequest'

const makeMemory = (overrides: Partial<Memory> = {}): Memory => ({
  id: 'mem-1',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-alpha',
  project: null,
  text: 'Sample memory',
  title: null,
  status: 'active',
  metadata: {},
  labels: [],
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
  const navigateMock = vi.fn()
  const onDeleteMock = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
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

describe('the Content column cell', () => {
  const renderContentCell = (memory: Memory) => {
    const columns = buildMemoriesColumns({
      navigate: vi.fn(),
      onDelete: vi.fn(),
      canDelete: () => true,
      includeTags: false,
    })
    const contentColumn = columns.find(
      c => 'accessorKey' in c && c.accessorKey === 'text'
    )
    const renderCell = contentColumn?.cell
    if (typeof renderCell !== 'function') {
      throw new Error('the Content column has no cell renderer')
    }
    render(
      <MemoryRouter>
        {renderCell({
          row: { original: memory },
        } as Parameters<typeof renderCell>[0])}
      </MemoryRouter>
    )
  }

  it('renders an untitled memory by a body-derived name, as plain text', () => {
    renderContentCell(makeMemory({ text: '**bold** body with `code`' }))

    const cell = screen.getByText(/bold/)
    expect(cell.textContent).toBe('bold body with code')
    // Subdued: a stand-in, not a title someone chose.
    expect(cell).toHaveClass('text-muted-foreground')
  })

  it('names an untitled memory by its first heading, as the detail page does', () => {
    renderContentCell(makeMemory({ text: '## Heading\n\n**bold** body' }))

    expect(screen.getByRole('link', { name: 'Heading' })).toBeInTheDocument()
  })

  it('links the name to the memory (a real href, not a scripted button)', () => {
    renderContentCell(makeMemory({ id: 'mem-9', title: 'Deploy checklist' }))

    expect(
      screen.getByRole('link', { name: 'Deploy checklist' })
    ).toHaveAttribute('href', '/memories/mem-9')
  })

  it('shows the title when the memory has one (#911)', () => {
    renderContentCell(
      makeMemory({ title: 'Deploy checklist', text: 'Drain the node first.' })
    )

    const cell = screen.getByText('Deploy checklist')
    expect(cell).not.toHaveClass('text-muted-foreground')
    expect(screen.queryByText(/Drain the node/)).not.toBeInTheDocument()
  })

  it.each([['   '], ['']])(
    'falls back to the body excerpt for a title of %j',
    title => {
      // The schema puts no `minLength` on `title`, so an empty string is a
      // storable value — and `??` would render it as a blank cell.
      renderContentCell(makeMemory({ title, text: 'Drain the node first.' }))

      expect(screen.getByText('Drain the node first.')).toBeInTheDocument()
    }
  )

  it('names a blank memory "Untitled memory" rather than an ellipsis', () => {
    renderContentCell(makeMemory({ id: 'mem-blank', text: '   \n  ' }))

    expect(screen.queryByText('…')).not.toBeInTheDocument()
    // Still a link, so even a blank memory can be opened from the list.
    expect(
      screen.getByRole('link', { name: 'Untitled memory' })
    ).toHaveAttribute('href', '/memories/mem-blank')
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
