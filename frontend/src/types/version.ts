// Resource-agnostic content-versioning types.
//
// The backend versioning core is polymorphic — a snapshot is keyed by
// `(resource_type, resource_id)` in `content_versions` — so the frontend models
// versions generically too. These types are sourced from the generated OpenAPI
// schema (`@vibexp/api-client`); the wire shape is `ContentVersion`. See backend
// `internal/models/content_version.go` for the source of truth.

import type { components } from '@vibexp/api-client'

// How a version came to be: a human edit or a system action (e.g. a restore or a
// future auto-save). Mirrors `models.ActorType{Human,System}`.
export type VersionActorType =
  components['schemas']['ContentVersion']['actor_type']

// Resolved, render-ready attribution for a version's author, populated
// server-side from `created_by` so clients don't issue per-version user lookups.
// The generated `VersionAuthor` is nullable at the schema level (a version may
// have no resolvable author, carried by `ContentVersion.author`); this alias
// models the populated object.
export type VersionAuthor = NonNullable<components['schemas']['VersionAuthor']>

// A single immutable snapshot of a versioned resource's content.
export type ContentVersion = components['schemas']['ContentVersion']

// Back-compat alias: the resource type files (artifact/blueprint/memory/prompt)
// and the version-history feature module still reference `ResourceVersion`.
export type ResourceVersion = ContentVersion

// Wire shape of the version-listing endpoints: a single object with a
// newest-first `versions` array. The spec exposes four per-resource names
// (`ArtifactVersionListResponse`, …), all `{ versions: ContentVersion[] }`; this
// generic local alias serves the resource-agnostic version-history module.
export interface ResourceVersionListResponse {
  versions: ContentVersion[]
}
