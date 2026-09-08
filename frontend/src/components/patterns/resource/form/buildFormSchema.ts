import { z } from 'zod'

import { fieldValues } from '../statusTone'
import type { FieldSpec, FormFieldSpec, ResourceDescriptor } from '../types'

/**
 * The zod schema for a resource's create/edit form, generated from
 * `descriptor.form`.
 *
 * Before #913 each of the three react-hook-form forms declared its own schema
 * and its own copy of the field rules, and they had already drifted: the same
 * slug regex was spelled three ways, `description` was capped at 500 on two
 * kinds and 200 on the third with no message on either, and "required" read
 * "Memory content is required" on one page and "Content is required" on the
 * next. The rules are descriptor data now, and the *messages* live here — one
 * per rule, for every kind — which is what makes them consistent by
 * construction rather than by review.
 *
 * ## Values
 *
 * Every control maps to one value type: the five scalar controls to a
 * `string`, `taxonomy` to `string[]`, `metadata` to the free-form object. That
 * is why {@link ResourceFormValues} can stay a plain record — the page reads
 * each value back through the control that produced it.
 *
 * Strings are `.trim()`ed, so the parsed values `handleSubmit` receives are
 * already normalized and a whitespace-only value fails its `required` check.
 * The three forms trimmed at four of their six call sites by hand.
 */

/** The one slug shape, for every kind. */
export const SLUG_PATTERN = /^[a-z0-9-]+$/

/** The one slug message, for every kind. */
export const SLUG_MESSAGE = 'Lowercase letters, numbers, and dashes only'

/** The values a generated form holds, keyed by field key. */
export type ResourceFormValues = Record<string, unknown>

/** "Title is required" — the one phrasing, built from the field's own label. */
export function requiredMessage(label: string): string {
  return `${label} is required`
}

/** "Description must be at most 500 characters". */
export function maxLengthMessage(label: string, maxLength: number): string {
  return `${label} must be at most ${String(maxLength)} characters`
}

/** "Each entry of Labels must be at most 50 characters". */
export function entryMaxLengthMessage(
  label: string,
  maxLength: number
): string {
  return `Each entry of ${label} must be at most ${String(maxLength)} characters`
}

/** "Labels allows at most 10 entries". */
export function maxItemsMessage(label: string, maxItems: number): string {
  return `${label} allows at most ${String(maxItems)} entries`
}

/**
 * The one `slugify`, lifted out of the three copies that had already drifted
 * (the prompt editor's kept spaces as a separate pass, the artifact and
 * blueprint pages disagreed on whether they had one at all).
 */
export function slugify(value: string): string {
  return (
    value
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      // The previous replace collapses runs, so at most one leading/trailing
      // dash remains — no quantifier needed (avoids super-linear backtracking).
      .replace(/^-|-$/g, '')
  )
}

/** The descriptor's fields by key. First declaration wins, as elsewhere. */
export function formFieldsByKey(
  descriptor: ResourceDescriptor
): ReadonlyMap<string, FieldSpec> {
  const byKey = new Map<string, FieldSpec>()
  for (const field of descriptor.fields) {
    if (!byKey.has(field.key)) byKey.set(field.key, field)
  }
  return byKey
}

/** The label a control and its validation messages read. */
export function formFieldLabel(
  byKey: ReadonlyMap<string, FieldSpec>,
  key: string
): string {
  return byKey.get(key)?.label ?? key
}

function scalarSchema(spec: FormFieldSpec, label: string): z.ZodType {
  let schema = z.string().trim()
  if (spec.required) schema = schema.min(1, requiredMessage(label))
  if (spec.maxLength !== undefined) {
    schema = schema.max(spec.maxLength, maxLengthMessage(label, spec.maxLength))
  }
  if (spec.pattern === 'slug') schema = schema.regex(SLUG_PATTERN, SLUG_MESSAGE)
  return spec.required ? schema : schema.optional()
}

/**
 * A `select` over an exhaustive vocabulary is an enum, so a value the API
 * cannot accept fails here rather than in a 400. A select over the team's
 * runtime type catalog cannot be — the vocabulary is not known at build time —
 * so it degrades to "some value was chosen", which is the same guarantee the
 * hand-written artifact form gave.
 */
function selectSchema(
  spec: FormFieldSpec,
  field: FieldSpec | undefined,
  label: string
): z.ZodType {
  const values = spec.optionsFrom === 'field' ? fieldValues(field) : []
  if (values.length === 0) return scalarSchema(spec, label)
  const schema = z.enum([...values] as [string, ...string[]])
  return spec.required ? schema : schema.optional()
}

/**
 * A label list is bounded on both axes by the API (`maxItems: 10`,
 * `items.maxLength: 50`), and those bounds were enforced in exactly one of the
 * four forms — as a hidden add-button in the prompt editor, with nothing behind
 * it. Declaring them on the descriptor is what turns "the UI happens to stop
 * you" into a rule every kind gets.
 */
function taxonomySchema(spec: FormFieldSpec, label: string): z.ZodType {
  const entry =
    spec.maxLength === undefined
      ? z.string()
      : z
          .string()
          .max(spec.maxLength, entryMaxLengthMessage(label, spec.maxLength))
  const list = z.array(entry)
  return spec.maxItems === undefined
    ? list
    : list.max(spec.maxItems, maxItemsMessage(label, spec.maxItems))
}

function controlSchema(
  spec: FormFieldSpec,
  field: FieldSpec | undefined,
  label: string
): z.ZodType {
  switch (spec.control) {
    case 'select':
      return selectSchema(spec, field, label)
    case 'taxonomy':
      return taxonomySchema(spec, label)
    case 'metadata':
      return z.record(z.string(), z.unknown())
    default:
      return scalarSchema(spec, label)
  }
}

/** The zod object a descriptor's form validates against. */
export function buildFormSchema(
  descriptor: ResourceDescriptor
): z.ZodType<ResourceFormValues, ResourceFormValues> {
  const byKey = formFieldsByKey(descriptor)
  const shape = new Map<string, z.ZodType>()
  for (const spec of descriptor.form?.fields ?? []) {
    shape.set(
      spec.key,
      controlSchema(spec, byKey.get(spec.key), formFieldLabel(byKey, spec.key))
    )
  }
  return z.object(Object.fromEntries(shape))
}

function scalarDefault(
  spec: FormFieldSpec,
  field: FieldSpec | undefined,
  current: unknown
): string {
  if (typeof current === 'string' && current !== '') return current
  // A select over an exhaustive vocabulary opens on its first value — which is
  // the default every hand-written form already picked (`active`, `draft`,
  // `general`), now read off the descriptor instead of retyped per page.
  if (spec.control === 'select' && spec.optionsFrom === 'field') {
    return fieldValues(field).at(0) ?? ''
  }
  return ''
}

function controlDefault(
  spec: FormFieldSpec,
  field: FieldSpec | undefined,
  current: unknown
): unknown {
  if (spec.control === 'taxonomy') {
    return Array.isArray(current)
      ? current.filter((value): value is string => typeof value === 'string')
      : []
  }
  if (spec.control === 'metadata') {
    const isMap =
      current !== null && typeof current === 'object' && !Array.isArray(current)
    return isMap ? current : {}
  }
  return scalarDefault(spec, field, current)
}

/**
 * Form values for a descriptor, seeded from an existing resource.
 *
 * Every declared field gets an entry of the right type whether or not the
 * resource carries it, because react-hook-form treats a field that starts
 * `undefined` as uncontrolled and React then warns the first time it is typed
 * into. Values are read through a `Map` rather than a computed index, the
 * house workaround for `security/detect-object-injection`.
 */
export function defaultFormValues(
  descriptor: ResourceDescriptor,
  initialValues?: ResourceFormValues
): ResourceFormValues {
  const byKey = formFieldsByKey(descriptor)
  const initial = new Map(Object.entries(initialValues ?? {}))
  const values = new Map<string, unknown>()
  for (const spec of descriptor.form?.fields ?? []) {
    values.set(
      spec.key,
      controlDefault(spec, byKey.get(spec.key), initial.get(spec.key))
    )
  }
  return Object.fromEntries(values)
}
