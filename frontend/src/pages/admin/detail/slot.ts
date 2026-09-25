import { getErrorMessage } from '@/utils/errorHandling'

/**
 * One async panel on an admin detail page: its value, whether it is in flight,
 * and its error. Each panel settles into its own slot, so one failing request
 * leaves the others rendered (#1137, #1146).
 */
export interface Slot<T> {
  data: T | null
  loading: boolean
  error: string | null
}

export const LOADING: Slot<never> = { data: null, loading: true, error: null }

/**
 * Settles `request` into a slot. `isCancelled` is the calling effect's cleanup
 * flag, so a late response for a superseded range is dropped.
 */
export function settle<T>(
  request: Promise<T>,
  set: (slot: Slot<T>) => void,
  fallback: string,
  isCancelled: () => boolean
): void {
  request
    .then(data => {
      if (!isCancelled()) set({ data, loading: false, error: null })
    })
    .catch((err: unknown) => {
      if (!isCancelled()) {
        set({
          data: null,
          loading: false,
          error: getErrorMessage(err, fallback),
        })
      }
    })
}
