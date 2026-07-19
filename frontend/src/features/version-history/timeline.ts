import { computeDiffStat } from './diff'
import type { VersionTimelineData, VersionTimelineEntry } from './types'

// Build the newest-first display timeline from the live draft + backend snapshots.
//
// The backend stores snapshots of PAST content (a row is recorded when its
// content is superseded), so the live draft is not itself a snapshot. We model it
// the way the rest of the app does (`currentVersion = maxSnapshot + 1`): a
// synthesized "Current" entry is prepended; the snapshots follow, each rendered
// directly as "Version {version_number}" with its own change summary.
// Each row's diffstat is computed against the next-older row; the oldest row has
// none (em-dash in the UI).
//
// "Changed by" attribution is SHIFTED UP one row. Under snapshot-on-write the
// author stamped on a snapshot is whoever *superseded* that content — i.e. the
// author of the *next newer* version — so reading a row's own `author` would show
// who replaced it, not who wrote it, and the current row (never superseded) would
// have no author at all (issue #398). Instead each row takes the author/actorType
// of the snapshot one position newer: the current row from the newest snapshot,
// snapshot vN from snapshot v(N+1). The oldest row then has no recorded author and
// renders "—" (its content is the originally-created content, whose creator the
// snapshot rows do not carry). Only the Changed-by chip (author + actorType)
// shifts; each row keeps its own change summary and timestamp.
export function buildTimeline(
  data: VersionTimelineData
): VersionTimelineEntry[] {
  const snapshots = [...data.versions].sort(
    (a, b) => b.version_number - a.version_number
  )
  const maxVersion = snapshots.length > 0 ? snapshots[0].version_number : 0

  const currentEntry: VersionTimelineEntry = {
    versionNumber: maxVersion + 1,
    content: data.currentContent,
    changeSummary: null,
    // Authored by whoever produced the current content = the actor recorded on
    // the newest snapshot (the one they created by superseding it).
    author: snapshots[0]?.author ?? null,
    actorType: snapshots[0]?.actor_type ?? null,
    createdAt: data.currentUpdatedAt,
    isCurrent: true,
    stat: null,
    restorable: false,
  }

  const snapshotEntries: VersionTimelineEntry[] = snapshots.map((v, index) => ({
    versionNumber: v.version_number,
    content: v.content,
    changeSummary: v.change_summary,
    // Authored by whoever produced THIS content = the actor on the next-newer
    // snapshot; the oldest snapshot has none (original content), so "—".
    author: snapshots[index + 1]?.author ?? null,
    actorType: snapshots[index + 1]?.actor_type ?? null,
    createdAt: v.created_at,
    isCurrent: false,
    stat: null,
    restorable: true,
  }))

  const ordered = [currentEntry, ...snapshotEntries]

  // Fill diffstats: each entry vs the next-older entry's content.
  return ordered.map((entry, index) => {
    const hasOlder = index + 1 < ordered.length
    return {
      ...entry,
      stat: hasOlder
        ? computeDiffStat(ordered[index + 1].content, entry.content)
        : null,
    }
  })
}
