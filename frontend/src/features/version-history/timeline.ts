import { computeDiffStat } from './diff'
import type { VersionTimelineData, VersionTimelineEntry } from './types'

// Build the newest-first display timeline from the live draft + backend snapshots.
//
// The backend stores snapshots of PAST content (a row is recorded when its
// content is superseded), so the live draft is not itself a snapshot. We model it
// the way the rest of the app does (`currentVersion = maxSnapshot + 1`): a
// synthesized "Current" entry is prepended; the snapshots follow, each rendered
// directly as "Version {version_number}" with its own change summary + author.
// Each row's diffstat is computed against the next-older row; the oldest row has
// none (em-dash in the UI).
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
    author: null,
    actorType: null,
    createdAt: data.currentUpdatedAt,
    isCurrent: true,
    stat: null,
    restorable: false,
  }

  const snapshotEntries: VersionTimelineEntry[] = snapshots.map(v => ({
    versionNumber: v.version_number,
    content: v.content,
    changeSummary: v.change_summary,
    author: v.author,
    actorType: v.actor_type,
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
