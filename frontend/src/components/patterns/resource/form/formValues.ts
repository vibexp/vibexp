import type { ResourceFormValues } from './buildFormSchema'

/*
 * Reading a generated form's parsed values back out, on the way to a request
 * body.
 *
 * `ResourceFormValues` is a `Record<string, unknown>` because the page reads
 * each value back through the control that produced it (`buildFormSchema`) —
 * which leaves every create/edit page needing the same four narrowings to build
 * its `Create*Request`. Four pages spelling those out inline is the duplication
 * `formSpecs.ts` and `filterSpecs.ts` already record the lesson for, and the
 * `security/detect-object-injection` workaround (read through a `Map`, never a
 * computed index) would be copied four times with it.
 *
 * These are narrowings of values the generated zod schema has ALREADY
 * validated, not a fresh trust boundary: a missing or wrong-typed value here
 * means the descriptor and the page disagree about a field key, and the safe
 * empty answer is what the old hand-written forms produced too.
 */

function valueOf(values: ResourceFormValues, key: string): unknown {
  return new Map(Object.entries(values)).get(key)
}

/** A `text` / `textarea` / `body` / `select` / `project` value. */
export function stringValue(values: ResourceFormValues, key: string): string {
  const value = valueOf(values, key)
  return typeof value === 'string' ? value : ''
}

/** A `taxonomy` value — the chip list, always present, possibly empty. */
export function stringListValue(
  values: ResourceFormValues,
  key: string
): string[] {
  const value = valueOf(values, key)
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === 'string')
}

/** A `metadata` value — the free-form bag, always present, possibly empty. */
export function recordValue(
  values: ResourceFormValues,
  key: string
): Record<string, unknown> {
  const value = valueOf(values, key)
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }
  return value as Record<string, unknown>
}

/**
 * A `select` value, narrowed to the literal union its request field declares.
 *
 * The generated schema is a `z.enum` over the descriptor's own `statusValues`,
 * and `resourceListSpec.test.ts` pins those to the endpoint's enum — so the
 * value is already one of `allowed`. Passing the tuple typed as the request
 * field's own type is what turns a spec change into a compile error here rather
 * than a 400 at runtime; the fallback is unreachable and exists so this needs
 * no type assertion.
 */
export function enumValue<T extends string>(
  values: ResourceFormValues,
  key: string,
  allowed: readonly [T, ...T[]]
): T {
  const current = stringValue(values, key)
  return allowed.find(value => value === current) ?? allowed[0]
}

/**
 * A metadata bag as the API wants it: omitted entirely when empty, which is
 * what all three hand-written forms did and what keeps an untouched resource's
 * `metadata` out of its update payload.
 */
export function metadataOrUndefined(
  metadata: Record<string, unknown>
): Record<string, unknown> | undefined {
  return Object.keys(metadata).length > 0 ? metadata : undefined
}
