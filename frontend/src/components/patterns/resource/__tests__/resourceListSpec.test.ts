import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import type { ResourceKindKey } from '../registry'
import { getResourceDescriptor } from '../registry'
import type { FieldRole } from '../types'
import { paramChunks, queryEnum, SPEC_DIR, systemStatuses } from './specYaml'

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

  it.each(KINDS)(
    '%s is sortable by the field its primary column shows, and by updated_at',
    kind => {
      const descriptor = getResourceDescriptor(kind)
      // The primary column reads the `name` field on every kind but memory:
      // a memory's title is optional (#911) so the column shows an excerpt of
      // the BODY, and `sort_by` on /memories offers `text`, never `title`.
      const role: FieldRole = kind === 'memory' ? 'body' : 'name'
      const key = descriptor.fields.find(field => field.role === role)?.key
      expect(descriptor.list?.sortable).toContain(key)
      expect(descriptor.list?.sortable).toContain('updated_at')
    }
  )

  it.each(KINDS)('%s offers a search and a freshness filter', kind => {
    const controls = (getResourceDescriptor(kind).list?.filters ?? []).map(
      filter => filter.control
    )
    expect(controls).toContain('search')
    expect(controls).toContain('freshness')
  })

  // A filter's key IS the query parameter it drives, so one the list endpoint
  // does not declare is a control that silently filters nothing.
  it.each(KINDS)(
    '%s only declares filters its list endpoint has a parameter for',
    (kind, file) => {
      const yaml = readFileSync(resolve(SPEC_DIR, file), 'utf8')
      const declared = (getResourceDescriptor(kind).list?.filters ?? []).map(
        filter => filter.key
      )
      expect(declared.length).toBeGreaterThan(0)
      expect(
        declared.filter(key => paramChunks(yaml, key).length === 0)
      ).toEqual([])
    }
  )

  // Lock state exists on prompts, blueprints and artifacts only (#1285), and
  // the spec is what says so: a kind offers the filter exactly when its list
  // endpoint takes a boolean `locked`. That keeps memories without one, and
  // fails if a lockable kind loses its filter.
  it.each(KINDS)(
    '%s offers a lock filter exactly when its list endpoint takes `locked`',
    (kind, file) => {
      const yaml = readFileSync(resolve(SPEC_DIR, file), 'utf8')
      const chunks = paramChunks(yaml, 'locked')
      for (const chunk of chunks) expect(chunk).toContain('type: boolean')
      const lock = (getResourceDescriptor(kind).list?.filters ?? []).filter(
        filter => filter.control === 'lock'
      )
      expect(lock.map(filter => filter.key)).toEqual(
        chunks.length > 0 ? ['locked'] : []
      )
      expect(chunks.length > 0).toBe(kind !== 'memory')
    }
  )

  // A descriptor's status/type values stopped being badge trivia in #908: they
  // are now ALSO the page's request guard, so a value the backend adds and the
  // descriptor lacks is stripped from the request — `?status=<new>` silently
  // returns an unfiltered list. Hand review caught that drift twice on this
  // branch, which is the evidence that review is the wrong mechanism for it.
  //
  // Since #1266 the list's `status` parameter is an open string: it also
  // accepts the team's custom statuses, which no spec can enumerate. The
  // descriptor still declares the SYSTEM statuses only, so that is what this
  // pins. Offering a team's custom statuses is #1270.
  it.each(KINDS)(
    "%s's status values are exactly its list endpoint's system statuses",
    (kind, file) => {
      const system = systemStatuses(file)
      expect(system.size).toBeGreaterThan(0)
      expect([...declaredValues(kind, 'status')].sort()).toEqual(
        [...system].sort()
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
