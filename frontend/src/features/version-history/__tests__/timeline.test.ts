import type { ResourceVersion, VersionAuthor } from '@/types/version'

import { buildTimeline } from '../timeline'

function author(id: string): VersionAuthor {
  return {
    id,
    display_name: id,
    avatar_url: null,
    initials: id.slice(0, 2).toUpperCase(),
  }
}

function snapshot(
  n: number,
  content: string,
  authorId = 'user'
): ResourceVersion {
  return {
    id: `v${String(n)}`,
    team_id: 'team',
    resource_type: 'artifact',
    resource_id: 'res',
    version_number: n,
    content,
    change_summary: `summary ${String(n)}`,
    actor_type: 'human',
    created_by: authorId,
    author: author(authorId),
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

    // synthesized current = maxSnapshot + 1, not restorable, no own change summary
    expect(entries[0]).toMatchObject({
      versionNumber: 3,
      content: 'C3',
      isCurrent: true,
      restorable: false,
      changeSummary: null,
    })

    // snapshots render directly with their own summary and are restorable
    expect(entries[1]).toMatchObject({
      versionNumber: 2,
      content: 'C2',
      isCurrent: false,
      restorable: true,
      changeSummary: 'summary 2',
    })
    expect(entries[2].versionNumber).toBe(1)
  })

  it('shifts "Changed by" up one row so each row shows who authored its content (#398)', () => {
    // Content timeline: original C1 (creator unknown) → C2 by u2 → C3 (current) by u3.
    // Snapshot vN stores the content superseded by v(N+1)'s author, so:
    //   snapshot(2) holds C2 and is stamped u3 (u3 superseded C2 to make C3)
    //   snapshot(1) holds C1 and is stamped u2 (u2 superseded C1 to make C2)
    const entries = buildTimeline({
      currentContent: 'C3',
      currentUpdatedAt: '2026-06-12T10:00:00.000Z',
      resourceName: 'My artifact',
      versions: [snapshot(2, 'C2', 'u3'), snapshot(1, 'C1', 'u2')],
    })

    // current content C3 was authored by u3 (newest snapshot's actor), not "—"
    expect(entries[0].author?.id).toBe('u3')
    // content C2 was authored by u2 (next-older snapshot's actor)
    expect(entries[1].author?.id).toBe('u2')
    // original content C1 has no recorded creator → "—"
    expect(entries[2].author).toBeNull()
  })

  it('carries actorType alongside the shifted author so the persona renders on the right row', () => {
    const restore = snapshot(2, 'C2', 'u3')
    restore.actor_type = 'system' // C3 was produced by a system restore triggered by u3
    const entries = buildTimeline({
      currentContent: 'C3',
      currentUpdatedAt: '2026-06-12T10:00:00.000Z',
      resourceName: 'x',
      versions: [restore, snapshot(1, 'C1', 'u2')],
    })

    expect(entries[0].actorType).toBe('system')
    expect(entries[0].author?.id).toBe('u3')
    expect(entries[1].actorType).toBe('human')
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

  it('handles a resource with no snapshots (current only) with no author', () => {
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
    // no snapshot exists, so the current (originally-created) content has no
    // recorded author → "—"
    expect(entries[0].author).toBeNull()
  })
})
