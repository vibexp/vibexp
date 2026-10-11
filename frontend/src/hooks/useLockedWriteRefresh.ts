import { useCallback, useState } from 'react'

import type { ResourceLock } from '@/components/LockIndicator'
import { isLockedWrite } from '@/utils/lockedWrite'

interface Lockable {
  id: string
  lock: ResourceLock | null
}

/**
 * The edit pages' half of the lock race (#1285): a resource locked AFTER the
 * page loaded. The page still offers every field, the save comes back 423, and
 * the page has to learn about the lock without a reload.
 *
 * `refreshOnLockedWrite` refetches the resource when — and only when — the
 * error is that 423. The result is kept beside the loaded resource rather than
 * replacing it, because the loaded resource is the form's seed and replacing
 * it would re-seed the form, discarding whatever the reader was typing. Pass
 * `lock` and `record` to `ResourceFormReadingPage`, which freezes the fields
 * and shows their stored values.
 */
export function useLockedWriteRefresh<T extends Lockable>(
  loaded: T | null,
  refetch: () => Promise<T>
) {
  const [refetched, setRefetched] = useState<T | null>(null)
  // A refetch belongs to the resource it was made for; the page can be reused
  // for another one without unmounting.
  const record = refetched && refetched.id === loaded?.id ? refetched : null

  const refreshOnLockedWrite = useCallback(
    async (error: unknown): Promise<T | null> => {
      if (!isLockedWrite(error)) return null
      try {
        const fresh = await refetch()
        setRefetched(fresh)
        return fresh
      } catch {
        // The refused save has already been explained to the reader. A failed
        // refetch leaves the page as it was, and the next save is refused with
        // the same message.
        return null
      }
    },
    [refetch]
  )

  return {
    lock: (record ?? loaded)?.lock ?? null,
    /** The refetched resource, or undefined until a 423 caused a refetch. */
    record: record ?? undefined,
    refreshOnLockedWrite,
  }
}
