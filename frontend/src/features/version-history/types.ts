import type {
  ResourceVersion,
  VersionActorType,
  VersionAuthor,
} from '@/types/version'

import type { DiffStat } from './diff'

// One row in the version-history timeline. It wraps either a real backend
// snapshot or the synthesized "Current" live-draft entry, so the list, compare
// and restore surfaces all speak the same shape regardless of resource type.
export interface VersionTimelineEntry {
  versionNumber: number
  content: string
  changeSummary: string | null
  author: VersionAuthor | null
  actorType: VersionActorType | null
  createdAt: string
  // The live draft (newest). Tagged "Current"; not itself a restorable snapshot.
  isCurrent: boolean
  // diffstat of this entry vs the next-older entry; null for the oldest row.
  stat: DiffStat | null
  // true when this entry maps to a real, restorable backend snapshot.
  restorable: boolean
}

// Everything the generic page needs to render a resource's history. The live
// draft is supplied separately because, under the snapshot-on-write model, it is
// not yet a persisted version.
export interface VersionTimelineData {
  currentContent: string
  currentUpdatedAt: string
  resourceName: string
  versions: ResourceVersion[]
}

// The resource-agnostic descriptor a consumer hands to `VersionHistoryPage`.
// Artifacts are the first adopter (see `versionService.createArtifactVersionSource`);
// adding Prompts/Blueprints/Memory later is a new source + route, not a rewrite.
export interface VersionHistorySource {
  // Matches `content_versions.resource_type` (e.g. "artifact").
  resourceType: string
  // Singular noun for copy, e.g. "artifact" → "Back to artifact".
  resourceLabel: string
  // react-router target for the "Back to <resource>" button.
  backHref: string
  // Loads the live draft + its versions (newest-first).
  load: () => Promise<VersionTimelineData>
  // Restores the given snapshot version (non-destructive; backend snapshots the
  // current draft first).
  restore: (versionNumber: number) => Promise<void>
}

export type { ResourceVersion }
