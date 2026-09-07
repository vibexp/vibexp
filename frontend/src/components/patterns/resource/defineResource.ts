import type { FieldSpec, ResourceDescriptor } from './types'

function fail(kind: string, message: string): never {
  throw new Error(`defineResource(${kind}): ${message}`)
}

function countRole(fields: readonly FieldSpec[], role: FieldSpec['role']) {
  return fields.filter(f => f.role === role).length
}

/**
 * A field satisfies a route segment when it names it exactly (`slug`, `id`) or
 * names it with an `_id` suffix — `/artifacts/:project/:slug` is built from
 * `artifact.project_id`, not from a `project` property.
 */
function matchesSegment(key: string, segment: string) {
  return key === segment || key === `${segment}_id`
}

/**
 * Validates a resource descriptor and returns it unchanged.
 *
 * Validation runs at module load — unconditionally, not behind
 * `import.meta.env.DEV` — so a malformed descriptor fails the Vitest run and
 * the build rather than rendering a subtly wrong page.
 */
export function defineResource<T extends ResourceDescriptor>(descriptor: T): T {
  const { kind, fields, address } = descriptor

  // 1. Exactly one name — a resource with no human-readable identifier has no
  //    page heading, and two would leave the heading ambiguous.
  const names = countRole(fields, 'name')
  if (names !== 1) {
    fail(kind, `expected exactly one 'name' field, found ${String(names)}`)
  }

  // 2. At most one body. Zero is legitimate (a resource with nothing long-form).
  const bodies = countRole(fields, 'body')
  if (bodies > 1) {
    fail(kind, `expected at most one 'body' field, found ${String(bodies)}`)
  }

  // 3. No duplicate field. A key may repeat across roles (memory's `text` is
  //    both its name and its body); the same key in the same role is a mistake.
  const seen = new Set<string>()
  for (const field of fields) {
    const id = `${field.key}:${field.role}`
    if (seen.has(id)) {
      fail(kind, `duplicate field '${field.key}' with role '${field.role}'`)
    }
    seen.add(id)
  }

  // 4. The address fields must match the declared route shape, in order.
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

  // 5. Status metadata belongs to status fields, and every toned value must be
  //    one the resource can actually be in.
  for (const field of fields) {
    if (field.role !== 'status') {
      if (field.statusValues || field.tone) {
        fail(
          kind,
          `field '${field.key}' has role '${field.role}' but declares status values`
        )
      }
      continue
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

  return descriptor
}
