import { ApiError } from '../types/errors'

/** The code the API answers with when a lock refuses a write (#1289). */
export const RESOURCE_WRITE_LOCKED = 'RESOURCE_WRITE_LOCKED'

/**
 * Request field names that read badly as they are. Any other name is shown
 * with its underscores as spaces, so a field the server starts freezing later
 * still gets a readable message. The edit page's lock notice names the fields
 * through the same map, so the notice and the refusal cannot disagree.
 */
const FIELD_NAMES = new Map([
  ['project_id', 'project'],
  ['mcp_expose', 'MCP exposure'],
])

/**
 * Whether a lock refused this write: a 423 carrying `RESOURCE_WRITE_LOCKED`.
 *
 * Both are checked because 423 is also the status of `HAS_LOCKED_RESOURCES`
 * (a project delete or move blocked by locked resources inside it, #1290),
 * which has a different payload and its own wording.
 */
export function isLockedWrite(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.status === 423 &&
    error.code === RESOURCE_WRITE_LOCKED
  )
}

/** A frozen request field as the reader knows it: "project", "MCP exposure". */
export function lockedFieldName(field: string): string {
  return FIELD_NAMES.get(field) ?? field.replaceAll('_', ' ')
}

function textOf(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function lockerName(lockedBy: unknown): string {
  if (typeof lockedBy !== 'object' || lockedBy === null) return ''
  return textOf(new Map(Object.entries(lockedBy)).get('display_name'))
}

/**
 * What to tell the reader when a lock refused their write, or `null` when the
 * error is anything else.
 *
 * Built from the problem's `metadata` (`ResourceWriteLockedError` in
 * `backend/schemas/common.yaml`): the resource type, who holds the lock, their
 * comment, and the frozen `field` an update tried to change. `field` is absent
 * when a delete was refused. Metadata that does not have that shape falls back
 * to the server's own message, which is still better than a generic toast.
 */
export function lockedWriteMessage(error: unknown): string | null {
  if (!isLockedWrite(error)) return null

  const metadata = new Map(Object.entries(error.metadata ?? {}))
  const type = textOf(metadata.get('resource_type'))
  const name = lockerName(metadata.get('locked_by'))
  if (!type || !name) return error.getMessage()

  const comment = textOf(metadata.get('comment'))
  const lockedBy = comment
    ? `locked by ${name} ("${comment}")`
    : `locked by ${name}`
  const field = textOf(metadata.get('field'))
  if (!field) {
    return `This ${type} is ${lockedBy} and cannot be deleted until it is unlocked.`
  }
  return `The ${lockedFieldName(field)} of this ${type} cannot be changed: it is ${lockedBy}. Unlock it first.`
}
