import type {
  FieldRole,
  FieldSpec,
  FilterSpec,
  FormFieldSpec,
  ResourceAddressShape,
  ResourceDescriptor,
  ResourceFormSpec,
  ResourceListSpec,
} from './types'

function fail(kind: string, message: string): never {
  throw new Error(`defineResource(${kind}): ${message}`)
}

function countRole(fields: readonly FieldSpec[], role: FieldSpec['role']) {
  return fields.filter(f => f.role === role).length
}

/**
 * A resource with no human-readable identifier has no page heading, and two
 * would leave the heading ambiguous.
 */
function assertExactlyOneName(kind: string, fields: readonly FieldSpec[]) {
  const names = countRole(fields, 'name')
  if (names !== 1) {
    fail(kind, `expected exactly one 'name' field, found ${String(names)}`)
  }
}

/** Zero is legitimate — a resource may have nothing long-form to render. */
function assertAtMostOneBody(kind: string, fields: readonly FieldSpec[]) {
  const bodies = countRole(fields, 'body')
  if (bodies > 1) {
    fail(kind, `expected at most one 'body' field, found ${String(bodies)}`)
  }
}

/**
 * A key may repeat across roles — memory's `text` is both its name and its
 * body — so the identity of a field is the `(key, role)` pair.
 */
function assertNoDuplicateField(kind: string, fields: readonly FieldSpec[]) {
  const seen = new Set<string>()
  for (const field of fields) {
    const id = `${field.key}:${field.role}`
    if (seen.has(id)) {
      fail(kind, `duplicate field '${field.key}' with role '${field.role}'`)
    }
    seen.add(id)
  }
}

/**
 * A field satisfies a route segment when it names it exactly (`slug`, `id`) or
 * names it with an `_id` suffix — `/artifacts/:project/:slug` is built from
 * `artifact.project_id`, not from a `project` property.
 */
function matchesSegment(key: string, segment: string) {
  return key === segment || key === `${segment}_id`
}

/** The `address`-role fields must cover the route shape, in order. */
function assertAddressMatchesShape(
  kind: string,
  fields: readonly FieldSpec[],
  address: ResourceAddressShape
) {
  const addressFields = fields.filter(f => f.role === 'address')
  if (addressFields.length !== address.length) {
    fail(
      kind,
      `address ${JSON.stringify(address)} needs ${String(address.length)} 'address' field(s), found ${String(addressFields.length)}`
    )
  }
  addressFields.forEach((field, i) => {
    const segment = address.at(i) as string
    if (!matchesSegment(field.key, segment)) {
      fail(
        kind,
        `address segment '${segment}' expects a field keyed '${segment}' or '${segment}_id', found '${field.key}'`
      )
    }
  })
}

/**
 * Status metadata belongs to `status` fields, every status field declares its
 * values, and every toned value is one the resource can actually be in.
 *
 * The converse is deliberately not required: `tone` is optional and may be
 * partial, so a value without one falls back to the default badge.
 */
function assertStatusMetadata(kind: string, field: FieldSpec) {
  if (field.role !== 'status') {
    if (field.statusValues || field.tone) {
      fail(
        kind,
        `field '${field.key}' has role '${field.role}' but declares status metadata`
      )
    }
    return
  }
  const values = field.statusValues
  if (!values || values.length === 0) {
    fail(kind, `status field '${field.key}' declares no status values`)
  }
  for (const value of Object.keys(field.tone ?? {})) {
    if (!values.includes(value)) {
      fail(
        kind,
        `status value '${value}' is not one of ${JSON.stringify(values)}`
      )
    }
  }
}

/** An exhaustive value list only means anything on the field it classifies. */
function assertTypeValues(kind: string, field: FieldSpec) {
  if (field.typeValues && field.role !== 'type') {
    fail(
      kind,
      `field '${field.key}' declares 'typeValues' but has role '${field.role}' (only 'type' may)`
    )
  }
}

/**
 * `render` exists for the two blueprint/prompt `meta` rows that are not plain
 * scalars. Restricting it to `role: 'meta'` is what stops it becoming a
 * general per-page slot that re-creates the bespoke panels this replaces.
 */
function assertRenderIsMetaOnly(kind: string, field: FieldSpec) {
  if (field.render && field.role !== 'meta') {
    fail(
      kind,
      `field '${field.key}' declares 'render' but has role '${field.role}' (only 'meta' may)`
    )
  }
}

/**
 * `valueLabels` is display text for a closed-ish value set, so it only makes
 * sense on the two roles that render a value as a badge. On a `status` field
 * every labelled value must be one the resource can actually be in — the same
 * rule `tone` follows.
 */
function assertValueLabels(kind: string, field: FieldSpec) {
  const labels = field.valueLabels
  if (!labels) return
  if (field.role !== 'status' && field.role !== 'type') {
    fail(
      kind,
      `field '${field.key}' declares 'valueLabels' but has role '${field.role}' (only 'status' or 'type' may)`
    )
  }
  if (field.role !== 'status') return
  const values = field.statusValues ?? []
  for (const value of Object.keys(labels)) {
    if (!values.includes(value)) {
      fail(
        kind,
        `status value '${value}' is not one of ${JSON.stringify(values)}`
      )
    }
  }
}

/**
 * Timestamps every resource carries but no descriptor declares as a field —
 * `ResourceMetadataSection` renders the Created/Updated rows itself, so adding
 * them to `fields` would duplicate those rows. A sortable key may name one.
 */
const TIMESTAMP_SORT_KEYS: ReadonlySet<string> = new Set([
  'created_at',
  'updated_at',
])

/**
 * The `resource_type` values the metadata endpoints accept
 * (`backend/paths/metadata.yaml`). `ResourceFilterBar` addresses the catalog by
 * the descriptor's `plural`, which is a plain `string` — so without this a
 * `metadata` filter on a fifth kind would compile and then send a
 * `resource_type` the API rejects.
 */
const METADATA_RESOURCE_TYPES: ReadonlySet<string> = new Set([
  'artifacts',
  'blueprints',
  'memories',
])

/** Controls that drive a resource field, and so must name one. */
const FIELD_BACKED_CONTROLS: ReadonlySet<FilterSpec['control']> = new Set([
  'select',
  'taxonomy',
])

/**
 * The options source each field-backed control may read from. A `Map` rather
 * than a record because the lookup key is a runtime value, and a computed
 * index would trip `security/detect-object-injection` (which cannot be
 * suppressed in this tree).
 */
const ALLOWED_OPTIONS_SOURCES: ReadonlyMap<string, readonly string[]> = new Map(
  [
    ['select', ['field', 'types']],
    ['taxonomy', ['prompt-labels']],
  ]
)

function fieldsByKey(fields: readonly FieldSpec[]): Map<string, FieldSpec> {
  const byKey = new Map<string, FieldSpec>()
  for (const field of fields) {
    if (!byKey.has(field.key)) byKey.set(field.key, field)
  }
  return byKey
}

/**
 * A filter reading its options off the field can only do so when the field
 * enumerates them EXHAUSTIVELY. `valueLabels` deliberately does not count: it
 * is documented as partial, so an open `type` whose labels cover three of six
 * values would otherwise render a Select missing half the options — silently,
 * and only once the spec enum grows.
 */
function assertFieldOptions(
  kind: string,
  filter: FilterSpec,
  field: FieldSpec
) {
  if (filter.optionsFrom !== 'field') return
  if (!field.statusValues && !field.typeValues) {
    fail(
      kind,
      `filter '${filter.key}' reads options from its field, which enumerates none`
    )
  }
}

/**
 * `search`, `freshness` and `metadata` are list-level controls: they name no
 * field and have no option catalog. `select` and `taxonomy` need both.
 */
function assertFilter(
  kind: string,
  plural: string,
  filter: FilterSpec,
  byKey: ReadonlyMap<string, FieldSpec>
) {
  if (!FIELD_BACKED_CONTROLS.has(filter.control)) {
    if (filter.optionsFrom) {
      fail(
        kind,
        `filter '${filter.key}' has control '${filter.control}' but declares 'optionsFrom'`
      )
    }
    if (filter.control === 'metadata' && !METADATA_RESOURCE_TYPES.has(plural)) {
      fail(
        kind,
        `metadata filter needs a plural the metadata API knows, found '${plural}'`
      )
    }
    return
  }
  const field = byKey.get(filter.key)
  if (!field) {
    fail(
      kind,
      `filter '${filter.key}' names no declared field (control '${filter.control}')`
    )
  }
  const allowed = ALLOWED_OPTIONS_SOURCES.get(filter.control) ?? []
  if (!filter.optionsFrom || !allowed.includes(filter.optionsFrom)) {
    fail(
      kind,
      `filter '${filter.key}' with control '${filter.control}' needs optionsFrom ${JSON.stringify(allowed)}, found ${JSON.stringify(filter.optionsFrom ?? null)}`
    )
  }
  assertFieldOptions(kind, filter, field)
}

function assertFilters(
  kind: string,
  plural: string,
  filters: readonly FilterSpec[],
  byKey: ReadonlyMap<string, FieldSpec>
) {
  const seen = new Set<string>()
  for (const filter of filters) {
    if (seen.has(filter.key)) {
      fail(kind, `duplicate filter key '${filter.key}'`)
    }
    seen.add(filter.key)
    assertFilter(kind, plural, filter, byKey)
  }
}

/**
 * A sortable key becomes an accessor key on the list table and a `sort_by`
 * value on the request, so it must name something the resource actually has.
 */
function assertSortable(
  kind: string,
  sortable: readonly string[],
  byKey: ReadonlyMap<string, FieldSpec>
) {
  const seen = new Set<string>()
  for (const key of sortable) {
    if (seen.has(key)) fail(kind, `duplicate sortable key '${key}'`)
    seen.add(key)
    if (!byKey.has(key) && !TIMESTAMP_SORT_KEYS.has(key)) {
      fail(kind, `sortable key '${key}' names no declared field`)
    }
  }
}

function assertListSpec(
  kind: string,
  plural: string,
  fields: readonly FieldSpec[],
  list: ResourceListSpec | undefined
) {
  if (!list) return
  const byKey = fieldsByKey(fields)
  assertFilters(kind, plural, list.filters, byKey)
  assertSortable(kind, list.sortable, byKey)
}

/**
 * The roles each form control can serve.
 *
 * A control is a way of *entering* a kind of value, so it only fits a field
 * whose meaning matches: the body editor belongs on the long-form field, the
 * project picker on the field that holds a project id, an option list on a
 * field with a value vocabulary. Without this, a descriptor could put a Select
 * on a free-text title and the page would render an empty dropdown — the exact
 * silent-wrong-page failure the descriptor exists to prevent.
 *
 * Roles are matched against EVERY role the key carries, not just the first:
 * memory's `text` is both its `name` and its `body`, so its body control is
 * legal only if the lookup sees both.
 */
const CONTROL_ROLES: ReadonlyMap<string, readonly FieldRole[]> = new Map([
  ['text', ['name', 'address', 'summary', 'type', 'taxonomy', 'meta']],
  ['textarea', ['name', 'summary', 'body', 'meta']],
  ['body', ['body']],
  ['select', ['status', 'type']],
  ['project', ['address', 'meta']],
  ['taxonomy', ['taxonomy']],
  ['metadata', ['meta']],
])

/** The section each control must live in. Absent means "any section". */
const CONTROL_SECTION: ReadonlyMap<string, string> = new Map([
  ['body', 'body'],
  ['taxonomy', 'taxonomy'],
  ['metadata', 'taxonomy'],
])

/**
 * Controls that capture one value the user can leave empty, and so are the
 * only ones `required` means anything on. A chip list and a key/value bag are
 * always *present* (`[]`, `{}`) — marking one required would declare a rule
 * the form can neither show nor enforce.
 */
const REQUIRABLE_CONTROLS: ReadonlySet<string> = new Set([
  'text',
  'textarea',
  'body',
  'select',
  'project',
])

/** Every role the descriptor gives a key. See {@link CONTROL_ROLES}. */
function rolesByKey(
  fields: readonly FieldSpec[]
): ReadonlyMap<string, ReadonlySet<FieldRole>> {
  const roles = new Map<string, Set<FieldRole>>()
  for (const field of fields) {
    const existing = roles.get(field.key)
    if (existing) existing.add(field.role)
    else roles.set(field.key, new Set([field.role]))
  }
  return roles
}

/**
 * A `select` reads its options from the field's exhaustive list or from the
 * team's runtime type catalog — never from `valueLabels`, which is partial by
 * design, for the same reason a list filter may not (#908). A control with no
 * option list has no business declaring where options come from.
 */
function assertFormOptions(
  kind: string,
  form: FormFieldSpec,
  field: FieldSpec
) {
  if (form.control !== 'select') {
    if (form.optionsFrom) {
      fail(
        kind,
        `form field '${form.key}' has control '${form.control}' but declares 'optionsFrom'`
      )
    }
    return
  }
  if (!form.optionsFrom) {
    fail(
      kind,
      `form field '${form.key}' with control 'select' needs 'optionsFrom'`
    )
  }
  if (
    form.optionsFrom === 'field' &&
    !field.statusValues &&
    !field.typeValues
  ) {
    fail(
      kind,
      `form field '${form.key}' reads options from its field, which enumerates none`
    )
  }
}

/** The control fits the field's meaning, and sits where that control belongs. */
function assertFormControl(
  kind: string,
  form: FormFieldSpec,
  roles: ReadonlySet<FieldRole>
) {
  const allowed = CONTROL_ROLES.get(form.control) ?? []
  if (!allowed.some(role => roles.has(role))) {
    fail(
      kind,
      `form field '${form.key}' has control '${form.control}', which serves ${JSON.stringify(allowed)}, but the field has ${JSON.stringify([...roles])}`
    )
  }
  const section = CONTROL_SECTION.get(form.control)
  if (section && form.section !== section) {
    fail(
      kind,
      `form field '${form.key}' with control '${form.control}' belongs in the '${section}' section, found '${form.section}'`
    )
  }
  if (form.required && !REQUIRABLE_CONTROLS.has(form.control)) {
    fail(
      kind,
      `form field '${form.key}' is required but control '${form.control}' captures no required value`
    )
  }
  // An optional field left blank would fail its own pattern, so a patterned
  // field that is not required is a rule the user cannot satisfy by omission.
  if (form.pattern && !form.required) {
    fail(
      kind,
      `form field '${form.key}' declares pattern '${form.pattern}' but is not required`
    )
  }
}

function assertFormField(
  kind: string,
  form: FormFieldSpec,
  byKey: ReadonlyMap<string, FieldSpec>,
  roles: ReadonlyMap<string, ReadonlySet<FieldRole>>
) {
  const field = byKey.get(form.key)
  if (!field) {
    fail(
      kind,
      `form field '${form.key}' names no declared field (control '${form.control}')`
    )
  }
  assertFormControl(kind, form, roles.get(form.key) ?? new Set())
  assertFormOptions(kind, form, field)
}

/**
 * Extension slots are named so a page can fill one; two slots with one name
 * would make "which node goes here?" ambiguous, and an empty name is not
 * addressable at all.
 */
function assertFormExtensions(kind: string, extensions: readonly string[]) {
  const seen = new Set<string>()
  for (const name of extensions) {
    if (name.trim() === '') fail(kind, 'form declares an empty extension name')
    if (seen.has(name)) fail(kind, `duplicate form extension '${name}'`)
    seen.add(name)
  }
}

/**
 * A read-only kind is served without create/edit affordances at all, so a form
 * spec on one is dead data that would still have to be kept correct.
 */
function assertFormSpec(
  kind: string,
  fields: readonly FieldSpec[],
  form: ResourceFormSpec | undefined,
  readOnly: boolean | undefined
) {
  if (!form) return
  if (readOnly) fail(kind, 'a read-only resource declares a form spec')
  const byKey = fieldsByKey(fields)
  const roles = rolesByKey(fields)
  const seen = new Set<string>()
  let projects = 0
  for (const field of form.fields) {
    if (seen.has(field.key)) fail(kind, `duplicate form field '${field.key}'`)
    seen.add(field.key)
    if (field.control === 'project') projects += 1
    assertFormField(kind, field, byKey, roles)
  }
  if (projects > 1) {
    fail(
      kind,
      `expected at most one 'project' form control, found ${String(projects)}`
    )
  }
  assertFormExtensions(kind, form.extensions ?? [])
}

/**
 * `Object.freeze` is shallow, and a descriptor is a singleton every page holds
 * a reference to — one stray write would corrupt the app globally. The
 * `readonly` members on `ResourceDescriptor` stop that at compile time; this
 * stops it at runtime too, on every import path.
 */
function deepFreeze<T>(value: T): T {
  if (value !== null && typeof value === 'object') {
    Object.values(value).forEach(deepFreeze)
    Object.freeze(value)
  }
  return value
}

/**
 * Validates a resource descriptor and returns it, deeply frozen.
 *
 * Validation runs at module load — unconditionally, not behind
 * `import.meta.env.DEV` — so a malformed descriptor fails the Vitest run and
 * the build rather than rendering a subtly wrong page.
 */
export function defineResource<T extends ResourceDescriptor>(descriptor: T): T {
  const { kind, plural, fields, address, list, form, readOnly } = descriptor

  assertExactlyOneName(kind, fields)
  assertAtMostOneBody(kind, fields)
  assertNoDuplicateField(kind, fields)
  assertAddressMatchesShape(kind, fields, address)
  fields.forEach(field => {
    assertStatusMetadata(kind, field)
    assertTypeValues(kind, field)
    assertRenderIsMetaOnly(kind, field)
    assertValueLabels(kind, field)
  })
  assertListSpec(kind, plural, fields, list)
  assertFormSpec(kind, fields, form, readOnly)

  return deepFreeze(descriptor)
}
