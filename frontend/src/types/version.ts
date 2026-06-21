// Resource-agnostic content-versioning types.
//
// The backend versioning core is polymorphic — a snapshot is keyed by
// `(resource_type, resource_id)` in `content_versions` — so the frontend models
// versions generically too. Artifacts are the first adopter; Prompts, Blueprints
// and Memory reuse the same `ResourceVersion` shape later. See backend
// `internal/models/content_version.go` for the source of truth.

// How a version came to be: a human edit or a system action (e.g. a restore or a
// future auto-save). Mirrors `models.ActorType{Human,System}`.
export type VersionActorType = 'human' | 'system'

// Resolved, render-ready attribution for a version's author, populated
// server-side from `created_by` so clients don't issue per-version user lookups.
// Null when the version has no author or the user can no longer be resolved.
export interface VersionAuthor {
  id: string
  display_name: string
  avatar_url: string | null
  initials: string
}

// A single immutable snapshot of a versioned resource's content.
export interface ResourceVersion {
  id: string
  team_id: string
  resource_type: string
  resource_id: string
  version_number: number
  content: string
  // Optional human-readable "commit message" for the change captured at this
  // version (added in backend #1832; nullable).
  change_summary: string | null
  actor_type: VersionActorType
  created_by: string | null
  author: VersionAuthor | null
  created_at: string
}

// Wire shape of the version-listing endpoint: a single object with a
// newest-first `versions` array.
export interface ResourceVersionListResponse {
  versions: ResourceVersion[]
}
