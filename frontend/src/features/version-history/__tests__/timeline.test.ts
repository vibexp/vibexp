import type { ResourceVersion } from '@/types/version'

import { buildTimeline } from '../timeline'

function snapshot(n: number, content: string): ResourceVersion {
  return {
    id: `v${String(n)}`,
    team_id: 'team',
    resource_type: 'artifact',
    resource_id: 'res',
    version_number: n,
    content,
    change_summary: `summary ${String(n)}`,
    actor_type: 'human',
    created_by: 'user',
    author: {
      id: 'user',
      display_name: 'User',
      avatar_url: null,
      initials: 'U',
    },
    created_at: '2026-06-10T10:00:00.000Z',
  }
}

describe('buildTimeline', () => {
  it('prepends a synthesized current entry and renders snapshots directly', () => {
    const entries = buildTimeline({
      currentContent: 'C3',
      currentUpdatedAt: '2026-06-12T10:00:00.000Z',
      resourceName: 'My artifact',
      versions: [snapshot(2, 'C2'), snapshot(1, 'C1')],
    })

    expect(entries).toHaveLength(3)

    // synthesized current = maxSnapshot + 1, not restorable, no committed author
    expect(entries[0]).toMatchObject({
      versionNumber: 3,
      content: 'C3',
      isCurrent: true,
      restorable: false,
      changeSummary: null,
      author: null,
    })

    // snapshots render directly with their own summary/author and are restorable
    expect(entries[1]).toMatchObject({
      versionNumber: 2,
      content: 'C2',
      isCurrent: false,
      restorable: true,
      changeSummary: 'summary 2',
    })
    expect(entries[2].versionNumber).toBe(1)
  })

  it('computes per-row diffstat against the next-older row, none for the oldest', () => {
    const entries = buildTimeline({
      currentContent: 'a\nb\nc',
      currentUpdatedAt: '2026-06-12T10:00:00.000Z',
      resourceName: 'x',
      versions: [snapshot(2, 'a\nb'), snapshot(1, 'a')],
    })

    // current vs v2: +1 (c)
    expect(entries[0].stat).toEqual({ added: 1, removed: 0 })
    // v2 vs v1: +1 (b)
    expect(entries[1].stat).toEqual({ added: 1, removed: 0 })
    // oldest has nothing to diff against
    expect(entries[2].stat).toBeNull()
  })

  it('handles a resource with no snapshots (current only)', () => {
    const entries = buildTimeline({
      currentContent: 'only',
      currentUpdatedAt: '2026-06-12T10:00:00.000Z',
      resourceName: 'x',
      versions: [],
    })
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      versionNumber: 1,
      isCurrent: true,
      stat: null,
    })
  })
})
