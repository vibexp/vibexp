import type { FieldRole, FieldSpec, ResourceDescriptor } from './types'

/**
 * The descriptor's first field carrying `role`, or `undefined` when the kind has
 * none (a prompt has no `type`, a memory no `taxonomy`).
 *
 * A field key may carry more than one role — memory's `text` is both `name` and
 * `body` — so the lookup is by role and the first match wins, which is the order
 * the descriptor declares.
 */
export function fieldOfRole(
  descriptor: ResourceDescriptor,
  role: FieldRole
): FieldSpec | undefined {
  return descriptor.fields.find(field => field.role === role)
}
