import type { ResourceKindKey } from '../registry'
import { getResourceDescriptor } from '../registry'
import { queryEnum } from './specYaml'

/**
 * The descriptor's `list.sortable` becomes a `sort_by` value on the wire, and
 * every one of these endpoints answers a `sort_by` outside its enum with a 400
 * — so a key declared here that the API does not accept is a broken column
 * header, not a cosmetic mismatch.
 *
 * The enums are read out of the OpenAPI spec itself (see `specYaml.ts`) rather
 * than restated, so this fails when the backend narrows one rather than when
 * somebody remembers to update a copy.
 */

/** The path file whose `sort_by` enum governs each kind's list endpoint. */
const SPEC_FILE: Partial<Record<ResourceKindKey, string>> = {
  artifact: 'artifacts.yaml',
  blueprint: 'blueprints.yaml',
  memory: 'memories.yaml',
  prompt: 'prompts.yaml',
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
