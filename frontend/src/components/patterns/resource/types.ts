/**
 * A resource type described as data.
 *
 * Every list, detail and form page today hand-writes which fields it shows, so
 * the four built-in resources drift independently — the metadata rows alone
 * differ four ways. A `ResourceDescriptor` is the single source those shared
 * pages read instead, and it is the shape a user-defined resource type will be
 * delivered in later.
 *
 * Nothing renders from this module: it is pure data plus the validator in
 * `defineResource.ts`. Consumers arrive in the issues that follow #900.
 *
 * ## Field roles
 *
 * A role says what a field *means* to a page, not how it looks. A page asks the
 * descriptor for the `name` field rather than knowing that a prompt calls it
 * `name` and an artifact calls it `title`.
 *
 * | role | meaning |
 * | --- | --- |
 * | `name` | the human-readable identifier — page heading, primary list column |
 * | `address` | part of the detail route (see {@link ResourceAddressShape}) |
 * | `summary` | one-line description shown under the name |
 * | `body` | the long-form content the reading page renders |
 * | `status` | lifecycle state, rendered as a badge |
 * | `type` | the resource's own sub-classification (artifact/blueprint `type`) |
 * | `taxonomy` | free-form grouping — labels, tags, category |
 * | `meta` | everything else worth showing in the metadata panel |
 *
 * A field key may carry more than one role — memory's `text` is both its `name`
 * (lists render a truncated excerpt) and its `body` — so uniqueness is on the
 * `(key, role)` pair, not on `key` alone.
 *
 * ## Capabilities
 *
 * Which shared side panels a kind supports. They are facts about the resource
 * as it exists today (memory has no attachments; only prompts can be exposed
 * over MCP), not preferences a page may override.
 */

/** What a field means to a page. */
export type FieldRole =
  | 'name'
  | 'address'
  | 'summary'
  | 'body'
  | 'status'
  | 'type'
  | 'taxonomy'
  | 'meta'

/**
 * The badge tones `StatusBadge` understands, minus `default` — a declared
 * status value always names an explicit tone.
 */
export type FieldTone =
  'success' | 'warning' | 'destructive' | 'info' | 'neutral'

/** One field of a resource, as the pages need to know it. */
export interface FieldSpec {
  /** The property name on the API payload (`mcp_expose`, `project_id`, …). */
  key: string
  role: FieldRole
  /** Human-readable label for metadata rows, table headers and form fields. */
  label: string
  /** The API marks this property optional or nullable. */
  optional?: boolean
  /**
   * For a `status` field: every value the API can return, in display order.
   * Mirrors the per-kind enum in the OpenAPI spec until the shared
   * `ResourceStatus` enum lands (decision F).
   */
  statusValues?: readonly string[]
  /**
   * For a `status` field: the badge tone per status value. Every key must be
   * one of `statusValues`.
   */
  tone?: Readonly<Record<string, FieldTone>>
}

/** Which shared side panels a resource kind supports. */
export interface Capabilities {
  attachments: boolean
  versions: boolean
  comments: boolean
  relations: boolean
  mcp: boolean
}

/**
 * The segments of a resource's detail route, in order.
 *
 * `['slug']` → `/prompts/:slug`, `['project','slug']` →
 * `/artifacts/:project/:slug`, `['id']` → `/memories/:id`. The field carrying a
 * segment may name it either exactly (`slug`) or with an `_id` suffix
 * (`project_id`, which is what the `:project` segment actually holds).
 */
export type ResourceAddressShape =
  readonly ['slug'] | readonly ['project', 'slug'] | readonly ['id']

/** A resource type, described as data. */
export interface ResourceDescriptor {
  /** Stable discriminator; for team resources it is also the API resource type. */
  kind: string
  /** Noun for one of them ("prompt"). */
  singular: string
  /** Noun for several ("prompts"). */
  plural: string
  address: ResourceAddressShape
  fields: readonly FieldSpec[]
  capabilities: Capabilities
  /** No create/edit/delete affordances — the gallery is served read-only. */
  readOnly?: boolean
}
