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

/**
 * Every `sort_by` enum in a path file, unioned. A file describes both the
 * team-scoped and the by-project variant of the same list, and the descriptor
 * does not distinguish them.
 */
function sortByEnum(file: string): Set<string> {
  const yaml = readFileSync(resolve(SPEC_DIR, file), 'utf8')
  const matches = yaml.matchAll(/- name: sort_by[\s\S]*?enum: \[([^\]]+)\]/g)
  const values = new Set<string>()
  for (const [, list] of matches) {
    for (const value of list.split(',')) values.add(value.trim())
  }
  return values
}

const KINDS = Object.entries(SPEC_FILE) as [ResourceKindKey, string][]

describe('resource list specs', () => {
  it.each(KINDS)('%s declares a list section', kind => {
    expect(getResourceDescriptor(kind).list).toBeDefined()
  })

  it.each(KINDS)(
    '%s only declares sortable keys its list endpoint accepts',
    (kind, file) => {
      const accepted = sortByEnum(file)
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
