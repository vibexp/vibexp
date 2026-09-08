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
 *
 * ## List behaviour
 *
 * The optional `list` section says which controls the resource's list page
 * offers and which columns it may be sorted by, so a filter bar is generated
 * from the resource type rather than hand-written once per page (#908). Both
 * halves are validated against `fields` — and, for `sortable`, against the
 * list endpoint's own `sort_by` enum by `resourceListSpec.test.ts`, because a
 * key the API does not accept is a 400 on the first header click.
 */

import type { ReactNode } from 'react'

import type { StatusTone } from '@/components/StatusBadge'

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
 * status value always names an explicit tone. Derived rather than restated, so
 * a tone renamed on the badge fails to compile here.
 */
export type FieldTone = Exclude<StatusTone, 'default'>

/** One field of a resource, as the pages need to know it. */
export interface FieldSpec {
  /** The property name on the API payload (`mcp_expose`, `project_id`, …). */
  readonly key: string
  readonly role: FieldRole
  /** Human-readable label for metadata rows, table headers and form fields. */
  readonly label: string
  /** The API marks this property optional or nullable. */
  readonly optional?: boolean
  /**
   * For a `status` field: every value the API can return, in display order.
   * Mirrors the per-kind enum in the OpenAPI spec until the shared
   * `ResourceStatus` enum lands (decision F).
   */
  readonly statusValues?: readonly string[]
  /**
   * For a `status` field: the badge tone per status value. Every key must be
   * one of `statusValues`.
   */
  readonly tone?: Readonly<Record<string, FieldTone>>
  /**
   * Display text per raw value, for the `status` and `type` roles — the API
   * returns `work_reports`, the badge reads "Work reports". Partial by design:
   * a value with no entry renders as itself, so an `type` the server adds
   * later degrades to its raw string rather than to `undefined`.
   */
  readonly valueLabels?: Readonly<Record<string, string>>
  /**
   * Escape hatch for the handful of `meta` fields whose value is not a plain
   * scalar — a repo URL that renders as a link, a commit sha that renders
   * truncated, a boolean that reads "Exposed". Returning `null` renders no
   * row at all, which is how a field that belongs in another section (a
   * free-form metadata blob) opts out of the metadata list.
   *
   * Deliberately narrow: `defineResource` rejects it on any role but `meta`,
   * so it can never grow into a per-page layout slot.
   */
  readonly render?: (value: unknown) => ReactNode
}

/** Which shared side panels a resource kind supports. */
export interface Capabilities {
  readonly attachments: boolean
  readonly versions: boolean
  readonly comments: boolean
  readonly relations: boolean
  readonly mcp: boolean
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

/**
 * Which control a list filter renders as.
 *
 * `search`, `freshness` and `metadata` are list-level controls with no field
 * of their own — every resource searches over its own text, freshness is a
 * property of the freshness rules and `metadata` is the free-form blob — so
 * they name no `FieldSpec`. `select` and `taxonomy` do, and `defineResource`
 * enforces it.
 */
export type FilterControl =
  'search' | 'select' | 'freshness' | 'metadata' | 'taxonomy'

/**
 * Where a `select`/`taxonomy` filter's options come from.
 *
 * `field` reads them off the named `FieldSpec` (`statusValues` for a status,
 * `valueLabels` for a closed `type`), which is what stops the filter's option
 * list drifting from the badge's. The other two are catalogs only known at
 * runtime: `types` is the team's registered artifact types and `labels` is the
 * team's prompt label catalog.
 */
export type FilterOptionsSource = 'field' | 'types' | 'labels'

/** One control on a resource list's filter bar. */
export interface FilterSpec {
  /**
   * The URL/query parameter this control drives. For `select` and `taxonomy`
   * it must also be a declared field key.
   */
  readonly key: string
  readonly control: FilterControl
  /** Accessible name for the control ("Filter by status"). */
  readonly label: string
  /** Text of the "no filter" option ("All statuses"). `select` only. */
  readonly allLabel?: string
  /** Required on `select` and `taxonomy`, forbidden on the other controls. */
  readonly optionsFrom?: FilterOptionsSource
  /** Kept from the hand-written bars so existing page tests keep passing. */
  readonly testId?: string
}

/** How a resource's list page filters and sorts. */
export interface ResourceListSpec {
  /** The filter bar, in render order. */
  readonly filters: readonly FilterSpec[]
  /**
   * Column keys the list may be sorted by. Each is a declared field key or one
   * of the universal timestamps, and each must be accepted by the list
   * endpoint's `sort_by` enum.
   */
  readonly sortable: readonly string[]
}

/** A resource type, described as data. */
export interface ResourceDescriptor {
  /** Stable discriminator; for team resources it is also the API resource type. */
  readonly kind: string
  /** Noun for one of them ("prompt"). */
  readonly singular: string
  /** Noun for several ("prompts"). */
  readonly plural: string
  readonly address: ResourceAddressShape
  readonly fields: readonly FieldSpec[]
  readonly capabilities: Capabilities
  /** How the resource's list page filters and sorts. Absent for kinds with no list page. */
  readonly list?: ResourceListSpec
  /** No create/edit/delete affordances — the gallery is served read-only. */
  readonly readOnly?: boolean
}
