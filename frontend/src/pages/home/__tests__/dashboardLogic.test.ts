import type { FeedItem } from '@/services/feedService'
import type { Memory } from '@/services/memoryService'
import type { AccessCountByDate } from '@/services/resourceAccessService'

import {
  buildStaleGroups,
  type DashboardResource,
  mergeRecentChanges,
  type StaleResult,
  summarizeAccessChannels,
  toDashboardResource,
  unansweredFeedItems,
} from '../dashboardLogic'

function resource(
  overrides: Partial<DashboardResource> = {}
): DashboardResource {
  return {
    type: 'artifact',
    id: 'a1',
    name: 'Release checklist',
    nameIsFallback: false,
    url: '/artifacts/p1/release-checklist',
    userId: 'user-1',
    createdAt: '2026-10-01T10:00:00Z',
    updatedAt: '2026-10-01T10:00:00Z',
    ...overrides,
  }
}

describe('mergeRecentChanges', () => {
  it('merges every type into one list, newest update first', () => {
    const merged = mergeRecentChanges(
      [
        [resource({ id: 'a1', updatedAt: '2026-10-03T10:00:00Z' })],
        [
          resource({
            type: 'memory',
            id: 'm1',
            updatedAt: '2026-10-05T10:00:00Z',
          }),
          resource({
            type: 'memory',
            id: 'm2',
            updatedAt: '2026-10-01T10:00:00Z',
          }),
        ],
        [
          resource({
            type: 'prompt',
            id: 'p1',
            updatedAt: '2026-10-04T10:00:00Z',
          }),
        ],
      ],
      10
    )

    expect(merged.map(c => c.id)).toEqual(['m1', 'p1', 'a1', 'm2'])
  })

  it('keeps only the most recent `limit` changes', () => {
    const merged = mergeRecentChanges(
      [
        [
          resource({ id: 'old', updatedAt: '2026-09-01T00:00:00Z' }),
          resource({ id: 'new', updatedAt: '2026-10-09T00:00:00Z' }),
        ],
        [
          resource({
            type: 'blueprint',
            id: 'mid',
            updatedAt: '2026-10-02T00:00:00Z',
          }),
        ],
      ],
      2
    )

    expect(merged.map(c => c.id)).toEqual(['new', 'mid'])
  })

  it('calls a resource created until it has been edited', () => {
    const [edited, fresh, sameWrite] = mergeRecentChanges(
      [
        [
          resource({
            id: 'edited',
            createdAt: '2026-10-01T10:00:00Z',
            updatedAt: '2026-10-09T10:00:00Z',
          }),
          resource({
            id: 'fresh',
            createdAt: '2026-10-08T10:00:00Z',
            updatedAt: '2026-10-08T10:00:00Z',
          }),
          // The server stamps the two columns a moment apart on insert.
          resource({
            id: 'same-write',
            createdAt: '2026-10-07T10:00:00.000Z',
            updatedAt: '2026-10-07T10:00:00.900Z',
          }),
        ],
      ],
      10
    )

    expect(edited).toMatchObject({ id: 'edited', action: 'updated' })
    expect(fresh).toMatchObject({ id: 'fresh', action: 'created' })
    expect(sameWrite).toMatchObject({ id: 'same-write', action: 'created' })
  })

  it('orders identical timestamps deterministically', () => {
    const at = '2026-10-01T10:00:00Z'
    const lists = [
      [resource({ type: 'prompt', id: 'p', updatedAt: at })],
      [resource({ type: 'artifact', id: 'a', updatedAt: at })],
    ]

    expect(mergeRecentChanges(lists, 10).map(c => c.id)).toEqual(['a', 'p'])
    expect(mergeRecentChanges([...lists].reverse(), 10).map(c => c.id)).toEqual(
      ['a', 'p']
    )
  })

  it('is empty when nothing loaded', () => {
    expect(mergeRecentChanges([], 5)).toEqual([])
  })
})

describe('toDashboardResource.memory', () => {
  const memory = (overrides: Partial<Memory>): Memory =>
    ({
      id: 'm1',
      user_id: 'user-1',
      text: '# Runbook\n\nDrain the node first.',
      created_at: '2026-10-01T10:00:00Z',
      updated_at: '2026-10-01T10:00:00Z',
      ...overrides,
    }) as Memory

  it('names a memory through the shared display-name helper', () => {
    expect(
      toDashboardResource.memory(memory({ title: 'Deploy checklist' }))
    ).toMatchObject({
      name: 'Deploy checklist',
      nameIsFallback: false,
      url: '/memories/m1',
    })
    expect(toDashboardResource.memory(memory({ title: null }))).toMatchObject({
      name: 'Runbook',
      nameIsFallback: true,
    })
  })
})

describe('buildStaleGroups', () => {
  const stale = (overrides: Partial<StaleResult>): StaleResult => ({
    type: 'artifact',
    total: 1,
    resources: [resource()],
    ...overrides,
  })

  it('lists only the types that have something stale, in a fixed order', () => {
    const groups = buildStaleGroups(
      [
        stale({ type: 'prompt', total: 2 }),
        stale({ type: 'memory', total: 0, resources: [] }),
        stale({ type: 'artifact', total: 1 }),
      ],
      3
    )

    expect(groups.map(g => g.type)).toEqual(['artifact', 'prompt'])
  })

  it('links each group to its list pre-filtered to stale', () => {
    const [group] = buildStaleGroups([stale({ type: 'memory' })], 3)

    expect(group.listUrl).toBe('/memories?freshness=stale')
  })

  it('names the first few and counts the rest from the server total', () => {
    const [group] = buildStaleGroups(
      [
        stale({
          total: 12,
          resources: [
            resource({ id: '1' }),
            resource({ id: '2' }),
            resource({ id: '3' }),
            resource({ id: '4' }),
          ],
        }),
      ],
      3
    )

    expect(group.resources.map(r => r.id)).toEqual(['1', '2', '3'])
    expect(group.total).toBe(12)
    expect(group.more).toBe(9)
  })

  it('is empty when nothing is stale or nothing loaded', () => {
    expect(buildStaleGroups([], 3)).toEqual([])
    expect(buildStaleGroups([stale({ total: 0, resources: [] })], 3)).toEqual(
      []
    )
  })
})

describe('unansweredFeedItems', () => {
  const item = (overrides: Partial<FeedItem>): FeedItem =>
    ({
      id: 'f1',
      title: 'Post',
      ai_assistant_name: 'Claude Code',
      archived_at: null,
      posted_at: '2026-10-01T10:00:00Z',
      reply_count: 0,
      ...overrides,
    }) as FeedItem

  it('keeps active posts with zero replies, newest first', () => {
    const result = unansweredFeedItems([
      item({ id: 'older', posted_at: '2026-10-01T10:00:00Z' }),
      item({ id: 'newer', posted_at: '2026-10-05T10:00:00Z' }),
    ])

    expect(result.map(i => i.id)).toEqual(['newer', 'older'])
  })

  it('never lists a post that already has a reply', () => {
    // The API does not say who replied last, so one reply is enough to drop
    // it: "awaiting you" would be a guess.
    expect(
      unansweredFeedItems([item({ reply_count: 1 }), item({ reply_count: 7 })])
    ).toEqual([])
  })

  it('never lists an archived post', () => {
    expect(
      unansweredFeedItems([item({ archived_at: '2026-10-02T00:00:00Z' })])
    ).toEqual([])
  })
})

describe('summarizeAccessChannels', () => {
  const day = (overrides: Partial<AccessCountByDate>): AccessCountByDate => ({
    date: '2026-10-01',
    web: 0,
    cli: 0,
    mcp: 0,
    api: 0,
    total: 0,
    ...overrides,
  })

  it('sums each channel over the range', () => {
    expect(
      summarizeAccessChannels([
        day({ web: 30, mcp: 2, total: 32 }),
        day({ web: 11, cli: 1, api: 4, total: 16 }),
      ])
    ).toEqual({
      mcp: 2,
      cli: 1,
      api: 4,
      web: 41,
      total: 48,
      noAiToolReads: false,
    })
  })

  it('reports no AI-tool reads when only the web was used', () => {
    expect(
      summarizeAccessChannels([day({ web: 41, total: 41 })]).noAiToolReads
    ).toBe(true)
  })

  it.each([['mcp'], ['cli'], ['api']] as const)(
    'counts a single %s read as an AI-tool read',
    channel => {
      expect(
        summarizeAccessChannels([day({ web: 41, [channel]: 1 })]).noAiToolReads
      ).toBe(false)
    }
  )

  it('reports no AI-tool reads for an empty range too', () => {
    expect(summarizeAccessChannels([])).toEqual({
      mcp: 0,
      cli: 0,
      api: 0,
      web: 0,
      total: 0,
      noAiToolReads: true,
    })
  })
})
