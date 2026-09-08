import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Reading the OpenAPI spec from a frontend test.
 *
 * `backend/openapi.yaml` (bundled from `paths/` + `schemas/`) is the source of
 * truth for both sides of a descriptor↔API assertion (CLAUDE.md, "Spec-first
 * backend"), so these tests read it rather than restating it — a rule that
 * drifts is then a red test, not a 400 a user finds.
 *
 * Deliberately not a YAML parser: the spec is multi-file with cross-file
 * `$ref`s, no YAML dependency is in the frontend's tree, and every value these
 * assertions need is a scalar or a flat string list. What that costs is that
 * the readers must be *loud* when they find nothing — a silently empty answer
 * is what made the status assertions compare the wrong parameter for a week.
 */

export const SPEC_DIR = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../../backend/paths'
)

/** The `backend/schemas/` sibling of {@link SPEC_DIR}. */
export const SCHEMA_DIR = resolve(SPEC_DIR, '../schemas')

const REF = /\$ref: ['"]([^'"]+)['"]/

/** The values of an inline `enum: [a, b]` list. */
export function enumValues(list: string): string[] {
  return list.split(',').map(value => value.trim())
}

/**
 * The body of a YAML mapping key: every line after it indented deeper than it,
 * stopping at the first line that is not. Enough structure for a flat enum or
 * a property's scalar constraints.
 */
export function blockOf(yaml: string, name: string): string {
  const lines = yaml.split('\n')
  const start = lines.findIndex(line => line.trim() === `${name}:`)
  if (start === -1) return ''
  const indent = lines[start].length - lines[start].trimStart().length
  const body: string[] = []
  for (const line of lines.slice(start + 1)) {
    if (line.trim() === '') continue
    if (line.length - line.trimStart().length <= indent) break
    body.push(line)
  }
  return body.join('\n')
}

/**
 * The `enum` behind a `$ref`, following it through however many files it hops:
 * a `paths/*.yaml` parameter points at `openapi.yaml`, whose component is
 * itself a `$ref` into `schemas/common.yaml` (#912).
 *
 * Throws rather than returning nothing when a component cannot be resolved.
 */
export function refEnum(fromFile: string, ref: string, hops = 4): string[] {
  const [file, pointer] = ref.split('#')
  const target = resolve(dirname(fromFile), file)
  const name = pointer.split('/').filter(Boolean).at(-1) ?? ''
  const block = blockOf(readFileSync(target, 'utf8'), name)
  if (block === '') {
    throw new Error(`schema component '${name}' not found in ${target}`)
  }
  const inline = /enum: \[([^\]]+)\]/.exec(block)
  if (inline) return enumValues(inline[1])
  const next = REF.exec(block)
  if (!next || hops === 0) {
    throw new Error(`schema component '${name}' declares no enum (${target})`)
  }
  return refEnum(target, next[1], hops - 1)
}

/**
 * Every value a named query parameter accepts, unioned across a path file. A
 * file describes both the team-scoped and the by-project variant of the same
 * list, and a descriptor does not distinguish them.
 *
 * The parameter's own chunk is isolated FIRST — everything between its
 * `- name:` and the next one — before any `enum` is read out of it. Scanning
 * forward from the name for the first `enum:` instead is what broke when #912
 * hoisted the per-kind status enums behind a `$ref`: with no inline enum left
 * to find, the scan ran on into the NEXT parameter and compared each kind's
 * status values against its `sort_by` values.
 */
export function queryEnum(file: string, param: string): Set<string> {
  const path = resolve(SPEC_DIR, file)
  const yaml = readFileSync(path, 'utf8')
  const values = new Set<string>()
  for (const chunk of yaml.split(/^\s*- name: /m)) {
    if (!chunk.startsWith(`${param}\n`)) continue
    const inline = /enum: \[([^\]]+)\]/.exec(chunk)
    if (inline) {
      for (const value of enumValues(inline[1])) values.add(value)
      continue
    }
    const ref = REF.exec(chunk)
    if (ref) for (const value of refEnum(path, ref[1])) values.add(value)
  }
  return values
}

/** A property's declared bounds in a request schema. */
export interface PropertyLimits {
  maxLength?: number
  maxItems?: number
  itemMaxLength?: number
}

function numberIn(block: string, key: string): number | undefined {
  const match = new RegExp(`^\\s*${key}: (\\d+)`, 'm').exec(block)
  return match ? Number(match[1]) : undefined
}

/**
 * The bounds a create-request property declares — what a generated form field
 * must not be laxer than.
 *
 * `itemMaxLength` is the `items.maxLength` of an array property, which is how
 * the spec bounds a label list's entries.
 */
export function requestLimits(
  schemaFile: string,
  request: string,
  property: string
): PropertyLimits {
  const yaml = readFileSync(resolve(SCHEMA_DIR, schemaFile), 'utf8')
  const properties = blockOf(blockOf(yaml, request), 'properties')
  const block = blockOf(properties, property)
  if (block === '') {
    throw new Error(`${request}.${property} not found in ${schemaFile}`)
  }
  const items = blockOf(block, 'items')
  // The property's OWN scalars only: `blockOf` returns the whole subtree, so
  // reading `maxLength` off it would pick up the nested `items.maxLength` of an
  // array property and report a bound the property does not declare — a
  // silently wrong answer, which is the one failure mode this module exists to
  // rule out.
  const own = items === '' ? block : block.replace(items, '')
  return {
    maxLength: numberIn(own, 'maxLength'),
    maxItems: numberIn(own, 'maxItems'),
    itemMaxLength: numberIn(items, 'maxLength'),
  }
}
