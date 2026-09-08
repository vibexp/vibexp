import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import type { ResourceKindKey } from '../registry'
import { getResourceDescriptor } from '../registry'

/**
 * The descriptor's `list.sortable` becomes a `sort_by` value on the wire, and
 * every one of these endpoints answers a `sort_by` outside its enum with a 400
 * — so a key declared here that the API does not accept is a broken column
 * header, not a cosmetic mismatch.
 *
 * The enums are read out of the OpenAPI spec itself rather than restated, so
 * this fails when the backend narrows one rather than when somebody remembers
 * to update a copy. `backend/openapi.yaml` is the source of truth for both
 * sides of this assertion (CLAUDE.md, "Spec-first backend").
 */
const SPEC_DIR = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../../backend/paths'
)

/** The path file whose `sort_by` enum governs each kind's list endpoint. */
const SPEC_FILE: Partial<Record<ResourceKindKey, string>> = {
  artifact: 'artifacts.yaml',
  blueprint: 'blueprints.yaml',
  memory: 'memories.yaml',
  prompt: 'prompts.yaml',
}

/** Where a `$ref`'d schema component lives. */
const SCHEMA_DIR = resolve(SPEC_DIR, '../schemas')

/** The values of an inline `enum: [a, b]` list. */
function enumValues(list: string): string[] {
  return list.split(',').map(value => value.trim())
}

/**
 * The `enum` of a shared schema component, by name.
 *
 * Splitting on de-dented lines gives one chunk per top-level key, which is all
 * the structure this needs — the status components are flat string enums.
 */
function componentEnum(name: string): string[] {
  const yaml = readFileSync(resolve(SCHEMA_DIR, 'common.yaml'), 'utf8')
  const block = yaml
    .split(/\n(?=\S)/)
    .find(chunk => chunk.startsWith(`${name}:`))
  const inline = /enum: \[([^\]]+)\]/.exec(block ?? '')
  return inline ? enumValues(inline[1]) : []
}

/**
 * Every value a named query parameter accepts, unioned across a path file. A
 * file describes both the team-scoped and the by-project variant of the same
 * list, and the descriptor does not distinguish them.
 *
 * The parameter's own chunk is isolated FIRST — everything between its
 * `- name:` and the next one — before any `enum` is read out of it. Scanning
 * forward from the name for the first `enum:` instead is what broke when #912
 * hoisted the per-kind status enums into `common.yaml`: with no inline enum
 * left to find, the scan ran on into the NEXT parameter and compared each
 * kind's status values against its `sort_by` values. It failed loudly here,
 * but the same shape would just as easily have passed vacuously.
 */
function queryEnum(file: string, param: string): Set<string> {
  const yaml = readFileSync(resolve(SPEC_DIR, file), 'utf8')
  const values = new Set<string>()
  for (const chunk of yaml.split(/^\s*- name: /m)) {
    if (!chunk.startsWith(`${param}\n`)) continue
    const inline = /enum: \[([^\]]+)\]/.exec(chunk)
    if (inline) {
      for (const value of enumValues(inline[1])) values.add(value)
      continue
    }
    const ref = /\$ref: ['"][^'"]*#\/(?:components\/schemas\/)?(\w+)['"]/.exec(
      chunk
    )
    if (ref) for (const value of componentEnum(ref[1])) values.add(value)
  }
  return values
}

/** The values of the descriptor's field in a given role, in declaration order. */
function declaredValues(
  kind: ResourceKindKey,
  role: 'status' | 'type'
): readonly string[] {
  const field = getResourceDescriptor(kind).fields.find(f => f.role === role)
  return field?.statusValues ?? field?.typeValues ?? []
}

const KINDS = Object.entries(SPEC_FILE) as [ResourceKindKey, string][]

describe('resource list specs', () => {
  it.each(KINDS)('%s declares a list section', kind => {
    expect(getResourceDescriptor(kind).list).toBeDefined()
  })

  it.each(KINDS)(
    '%s only declares sortable keys its list endpoint accepts',
    (kind, file) => {
      const accepted = queryEnum(file, 'sort_by')
      // Guards the guard: a regex that matched nothing would make every
      // assertion below vacuous.
      expect(accepted.size).toBeGreaterThan(0)
      const declared = getResourceDescriptor(kind).list?.sortable ?? []
      expect(declared.length).toBeGreaterThan(0)
      expect(declared.filter(key => !accepted.has(key))).toEqual([])
    }
  )

  it.each(KINDS)('%s is sortable by its name field and by updated_at', kind => {
    const descriptor = getResourceDescriptor(kind)
    const nameKey = descriptor.fields.find(field => field.role === 'name')?.key
    expect(descriptor.list?.sortable).toContain(nameKey)
    expect(descriptor.list?.sortable).toContain('updated_at')
  })

  it.each(KINDS)('%s offers a search and a freshness filter', kind => {
    const controls = (getResourceDescriptor(kind).list?.filters ?? []).map(
      filter => filter.control
    )
    expect(controls).toContain('search')
    expect(controls).toContain('freshness')
  })

  // A descriptor's status/type values stopped being badge trivia in #908: they
  // are now ALSO the page's request guard, so a value the backend adds and the
  // descriptor lacks is stripped from the request — `?status=<new>` silently
  // returns an unfiltered list. Hand review caught that drift twice on this
  // branch, which is the evidence that review is the wrong mechanism for it.
  it.each(KINDS)(
    "%s's status values are exactly its list endpoint's status enum",
    (kind, file) => {
      const accepted = queryEnum(file, 'status')
      expect(accepted.size).toBeGreaterThan(0)
      expect([...declaredValues(kind, 'status')].sort()).toEqual(
        [...accepted].sort()
      )
    }
  )

  it("blueprint's type values are exactly its list endpoint's type enum", () => {
    // The one closed `type`. An artifact's is an open string matched against
    // the team's registered types, so it declares no exhaustive list at all.
    const accepted = queryEnum('blueprints.yaml', 'type')
    expect(accepted.size).toBeGreaterThan(0)
    expect([...declaredValues('blueprint', 'type')].sort()).toEqual(
      [...accepted].sort()
    )
    expect(declaredValues('artifact', 'type')).toEqual([])
  })

  it('offers a status filter on every kind that has a status', () => {
    for (const [kind] of KINDS) {
      const descriptor = getResourceDescriptor(kind)
      const hasStatus = descriptor.fields.some(field => field.role === 'status')
      const filtersStatus = (descriptor.list?.filters ?? []).some(
        filter => filter.key === 'status'
      )
      expect({ kind, hasStatus, filtersStatus }).toEqual({
        kind,
        hasStatus,
        filtersStatus: hasStatus,
      })
    }
  })
})
