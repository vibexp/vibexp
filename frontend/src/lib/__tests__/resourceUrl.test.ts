import { buildResourceUrl } from '@/lib/resourceUrl'

describe('buildResourceUrl', () => {
  it('builds a prompt URL from its slug', () => {
    expect(
      buildResourceUrl({ type: 'prompt', id: 'p1', slug: 'my-prompt' })
    ).toBe('/prompts/my-prompt')
  })

  it('builds artifact / blueprint URLs keyed by project id + slug', () => {
    expect(
      buildResourceUrl({
        type: 'artifact',
        id: 'a1',
        slug: 'q3-analysis',
        projectId: 'proj-1',
      })
    ).toBe('/artifacts/proj-1/q3-analysis')
    expect(
      buildResourceUrl({
        type: 'blueprint',
        id: 'b1',
        slug: 'onboarding',
        projectId: 'proj-2',
      })
    ).toBe('/blueprints/proj-2/onboarding')
  })

  it('builds a memory URL from its id', () => {
    expect(buildResourceUrl({ type: 'memory', id: 'mem-1' })).toBe(
      '/memories/mem-1'
    )
  })

  it('percent-encodes identifiers', () => {
    expect(buildResourceUrl({ type: 'prompt', id: 'p1', slug: 'a b+c' })).toBe(
      '/prompts/a%20b%2Bc'
    )
  })

  it('returns null when a required identifier is missing', () => {
    expect(buildResourceUrl({ type: 'prompt', id: 'p1' })).toBeNull()
    expect(
      buildResourceUrl({ type: 'artifact', id: 'a1', slug: 'x' })
    ).toBeNull() // missing projectId
    expect(
      buildResourceUrl({ type: 'blueprint', id: 'b1', projectId: 'proj-1' })
    ).toBeNull() // missing slug
  })

  it('returns null for an unknown resource type', () => {
    expect(buildResourceUrl({ type: 'agent', id: 'x1', slug: 's' })).toBeNull()
  })
})
