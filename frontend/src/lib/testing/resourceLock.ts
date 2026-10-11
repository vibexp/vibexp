import { ApiError } from '@/types/errors'
import type { ResourceLock } from '@/types/resourceLock'

/** A lock as the API returns it on a locked prompt, blueprint or artifact. */
export const TEST_LOCK: ResourceLock = {
  locked_by: { user_id: 'user-9', display_name: 'Ada Lovelace' },
  comment: 'Used by the nightly release agent',
  locked_at: '2026-10-11T10:15:30Z',
}

/**
 * The 423 the API answers a write refused by a lock with (#1289) — the shape
 * of `ResourceWriteLockedError` in `backend/schemas/common.yaml`. Pass
 * `metadata` to replace the documented one, e.g. to drop `field` as a refused
 * delete does.
 */
export function lockedWriteError(
  metadata: Record<string, unknown> = {
    resource_type: 'artifact',
    field: 'status',
    locked_by: TEST_LOCK.locked_by,
    comment: TEST_LOCK.comment,
  }
): ApiError {
  return new ApiError({
    type: 'https://api.vibexp.io/errors/RESOURCE_WRITE_LOCKED',
    title: 'Resource Locked',
    status: 423,
    detail: 'artifact is locked',
    code: 'RESOURCE_WRITE_LOCKED',
    request_id: 'req-423',
    timestamp: '2026-10-11T10:16:00Z',
    metadata,
  })
}
