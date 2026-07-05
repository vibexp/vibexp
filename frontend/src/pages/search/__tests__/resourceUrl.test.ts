import type { SearchResultItem } from '@/services/searchService'

import { displayTitle, resourceUrl } from '../resourceUrl'

function makeItem(overrides: Partial<SearchResultItem>): SearchResultItem {
  return {
    type: 'prompt',
    id: 'id-1',
    title: 'Title',
    excerpt: 'excerpt',
    score: 0.5,
    chunk_id: 'chunk-1',
    updated_at: '2024-01-01T00:00:00Z',
    slug: '',
    project_id: '',
    project_name: '',
    ...overrides,
  }
}

describe('displayTitle', () => {
  it('returns the type label for memory results', () => {
    expect(displayTitle(makeItem({ type: 'memory', title: 'ignored' }))).toBe(
      'Memory'
    )
  })

  it('returns the item title for non-memory results', () => {
    expect(
      displayTitle(makeItem({ type: 'prompt', title: 'Real Title' }))
    ).toBe('Real Title')
  })
})

describe('resourceUrl', () => {
  it('builds a prompt url from its slug', () => {
    expect(resourceUrl(makeItem({ type: 'prompt', slug: 'p' }))).toBe(
      '/prompts/p'
    )
  })

  it('builds an artifact url from project id (UUID) and slug', () => {
    expect(
      resourceUrl(
        makeItem({ type: 'artifact', slug: 'a', project_id: 'proj-uuid' })
      )
    ).toBe('/artifacts/proj-uuid/a')
  })

  it('builds a blueprint url from project id (UUID) and slug', () => {
    expect(
      resourceUrl(
        makeItem({ type: 'blueprint', slug: 'b', project_id: 'proj-uuid' })
      )
    ).toBe('/blueprints/proj-uuid/b')
  })

  it('builds a memory url from its id', () => {
    expect(resourceUrl(makeItem({ type: 'memory', id: 'mem-1' }))).toBe(
      '/memories/mem-1'
    )
  })

  it('returns null for a prompt missing its slug', () => {
    expect(resourceUrl(makeItem({ type: 'prompt', slug: '' }))).toBeNull()
  })

  it('returns null for an artifact missing its project id', () => {
    expect(
      resourceUrl(makeItem({ type: 'artifact', slug: 'a', project_id: '' }))
    ).toBeNull()
  })

  it('returns null for a blueprint missing its slug', () => {
    expect(
      resourceUrl(
        makeItem({ type: 'blueprint', slug: '', project_id: 'proj-uuid' })
      )
    ).toBeNull()
  })
})
