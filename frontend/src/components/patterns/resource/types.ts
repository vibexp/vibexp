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
 *
 * ## Form behaviour
 *
 * The optional `form` section says which fields a create/edit page edits, in
 * which control and in which of its three sections, so `ResourceFormPage`
 * generates the zod schema and the layout from the resource type rather than
 * each page hand-writing both (#913). Absent for kinds with no form.
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
   * For a closed `type` field: every value the API accepts, in display order.
   * The artifact `type` has none — it is an open string matched against the
   * team's registered types — which is exactly the difference a filter needs
   * to know about.
   */
  readonly typeValues?: readonly string[]
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
 * `field` reads them off the named `FieldSpec`'s exhaustive value list
 * (`statusValues` or `typeValues`), which is what stops the filter's option
 * list drifting from the badge's. Deliberately NOT `valueLabels`: that map is
 * partial by design, so promoting its keys to an option set would silently
 * hide any value the server later adds without a label.
 *
 * The other two are catalogs only known at runtime. `prompt-labels` names the
 * prompt label endpoint rather than a generic "labels", because that is the
 * only taxonomy catalog that exists — a generic name would invite a descriptor
 * to declare it for a taxonomy field it does not serve.
 */
export type FilterOptionsSource = 'field' | 'types' | 'prompt-labels'

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

/**
 * Which control a form field renders as.
 *
 * A closed union on purpose. The descriptor says what a field *is* — one line
 * of text, the long-form body, the project — and the form page owns how that
 * looks; a free-form control name would turn the descriptor into the per-page
 * layout DSL that `FieldSpec.render` is deliberately fenced off from being.
 *
 * | control | renders |
 * | --- | --- |
 * | `text` | single-line `Input`; the value is trimmed |
 * | `textarea` | short multi-line `Textarea` (a summary) |
 * | `body` | the long-form editor slot, a `Textarea` until #914 lands |
 * | `select` | option list, from the field's values or a runtime catalog |
 * | `project` | the shared `ProjectPicker` |
 * | `taxonomy` | a chip editor over a list of strings |
 * | `metadata` | the shared `MetadataEditor` over the free-form blob |
 *
 * Named `…Kind` rather than `FormControl` so it cannot be confused with — or
 * shadowed by — the shadcn `FormControl` primitive every form file imports.
 */
export type FormControlKind =
  'text' | 'textarea' | 'body' | 'select' | 'project' | 'taxonomy' | 'metadata'

/**
 * Where in the form a field sits.
 *
 * Three fixed names rather than coordinates: `details` is the identity card,
 * `body` is the wide editor column and `taxonomy` is the labels-and-metadata
 * card, matching the reading page's own division of the same resource. Order
 * inside a section is declaration order.
 */
export type FormSection = 'details' | 'body' | 'taxonomy'

/**
 * A named validation pattern, not a `RegExp`.
 *
 * `slug` is the only one, and it resolves to a single regex and a single
 * message in `buildFormSchema` — which is the point, because the three
 * hand-written forms had already drifted to three spellings of it. A literal
 * `RegExp` on the descriptor would also make it unserializable and put ReDoS
 * review on every future descriptor author.
 */
export type FormPattern = 'slug'

/**
 * Where a `select` control's options come from — the same two sources, and the
 * same reasoning, as {@link FilterOptionsSource}. `field` reads the named
 * field's exhaustive values; `types` is the team's registered type catalog,
 * only known at runtime, which is how the artifact `type` (an open string) is
 * expressible without the form page knowing what an artifact is.
 */
export type FormOptionsSource = 'field' | 'types'

/** One editable field on a resource's create/edit form. */
export interface FormFieldSpec {
  /** A declared {@link FieldSpec} key — the form edits fields, never invents them. */
  readonly key: string
  readonly control: FormControlKind
  readonly section: FormSection
  /** The value may not be empty. Anything else is `.optional()` in the schema. */
  readonly required?: boolean
  /** Max characters, mirroring the API's own limit. */
  readonly maxLength?: number
  /** Extra shape constraint beyond length. */
  readonly pattern?: FormPattern
  /** Placeholder text for the control. */
  readonly placeholder?: string
  /** Helper text under the control. */
  readonly description?: string
  /** Required on `select`, forbidden on every other control. */
  readonly optionsFrom?: FormOptionsSource
  /**
   * Editable while creating, locked while editing — an artifact's slug is part
   * of its address and changing it would break every link to it. Declared here
   * rather than hardcoded per page, because it is not uniform: a prompt's slug
   * stays editable.
   */
  readonly editableOnCreateOnly?: boolean
  /** Kept from the hand-written forms so the existing page and e2e tests keep passing. */
  readonly testId?: string
}

/** How a resource's create/edit page is laid out and validated. */
export interface ResourceFormSpec {
  /** The editable fields, in render order within their section. */
  readonly fields: readonly FormFieldSpec[]
  /**
   * Named slots a page may fill with a settings block the descriptor cannot
   * describe — prompt MCP exposure, the blueprint sub-agent required keys.
   * The form page renders whatever node it is handed under each name, in this
   * order, and knows nothing else about it. Declaring the names here is what
   * keeps the escape hatch enumerable instead of open-ended.
   */
  readonly extensions?: readonly string[]
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
  /** How the resource's create/edit page is laid out and validated. Absent for kinds with no form. */
  readonly form?: ResourceFormSpec
  /** No create/edit/delete affordances — the gallery is served read-only. */
  readonly readOnly?: boolean
}
