import type {
  FieldSpec,
  ResourceAddressShape,
  ResourceDescriptor,
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
  const { kind, fields, address } = descriptor

  assertExactlyOneName(kind, fields)
  assertAtMostOneBody(kind, fields)
  assertNoDuplicateField(kind, fields)
  assertAddressMatchesShape(kind, fields, address)
  fields.forEach(field => {
    assertStatusMetadata(kind, field)
  })

  return deepFreeze(descriptor)
}
