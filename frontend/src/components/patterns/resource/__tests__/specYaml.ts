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
  const name = pointer.split('/').findLast(Boolean) ?? ''
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

/** A schema component: the file that finally defines it, and its name there. */
export interface SchemaComponent {
  file: string
  name: string
}

/**
 * Where a `$ref` finally lands, following it through however many files it
 * hops (see {@link refEnum}): the last component whose block is a definition
 * rather than another `$ref`.
 *
 * Throws rather than returning nothing when a component cannot be resolved.
 */
export function refTarget(
  fromFile: string,
  ref: string,
  hops = 4
): SchemaComponent {
  const [file, pointer] = ref.split('#')
  const target = resolve(dirname(fromFile), file)
  const name = pointer.split('/').findLast(Boolean) ?? ''
  const block = blockOf(readFileSync(target, 'utf8'), name)
  if (block === '') {
    throw new Error(`schema component '${name}' not found in ${target}`)
  }
  const next = REF.exec(block)
  if (!next) return { file: target, name }
  if (hops === 0) {
    throw new Error(`schema component '${name}' never resolves (${target})`)
  }
  return refTarget(target, next[1], hops - 1)
}

/**
 * The one schema component a named query parameter `$ref`s, across a path
 * file's team-scoped and by-project variants of the same list.
 *
 * Throws when the parameter is missing, is declared inline, or its variants
 * point at different components — each of which would otherwise let a caller
 * compare against the wrong schema, or against nothing.
 */
export function queryComponent(file: string, param: string): SchemaComponent {
  const path = resolve(SPEC_DIR, file)
  const found = new Map<string, SchemaComponent>()
  for (const chunk of readFileSync(path, 'utf8').split(/^\s*- name: /m)) {
    if (!chunk.startsWith(`${param}\n`)) continue
    const ref = REF.exec(chunk)
    if (!ref) {
      throw new Error(`query parameter '${param}' has no $ref in ${path}`)
    }
    const component = refTarget(path, ref[1])
    found.set(`${component.file}#${component.name}`, component)
  }
  const components = [...found.values()]
  if (components.length !== 1) {
    throw new Error(
      `query parameter '${param}' resolves to ${String(components.length)} schema components in ${path}`
    )
  }
  return components[0]
}

/**
 * The SYSTEM status values of a per-kind status schema (`PromptStatus`,
 * `ArtifactStatus`, ...).
 *
 * Those schemas have been open strings since #1266 — a team's custom status is
 * a legal value — so there is no enum to read. What the spec still pins is the
 * system subset: each description names, in backticks, exactly the values of
 * the shared `ResourceStatus` vocabulary that are system statuses of that kind
 * (the backend holds it to that in `TestStatusSchemasAreOpenAndNameSystemValues`).
 * This reads the subset the same way, out of the file that defines the schema.
 *
 * Throws rather than returning nothing when the schema is missing, has grown
 * an enum again, or names no system value.
 */
export function systemStatuses({ file, name }: SchemaComponent): string[] {
  const yaml = readFileSync(file, 'utf8')
  const vocabulary = /enum: \[([^\]]+)\]/.exec(blockOf(yaml, 'ResourceStatus'))
  if (!vocabulary) {
    throw new Error(
      `schema component 'ResourceStatus' declares no enum (${file})`
    )
  }
  const block = blockOf(yaml, name)
  if (block === '') {
    throw new Error(`schema component '${name}' not found in ${file}`)
  }
  if (/^\s*enum:/m.test(block)) {
    throw new Error(
      `schema component '${name}' declares an enum; read it with refEnum (${file})`
    )
  }
  const named = enumValues(vocabulary[1]).filter(value =>
    block.includes('`' + value + '`')
  )
  if (named.length === 0) {
    throw new Error(
      `schema component '${name}' names no system status (${file})`
    )
  }
  return named
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
  const match = new RegExp(String.raw`^\s*${key}: (\d+)`, 'm').exec(block)
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
