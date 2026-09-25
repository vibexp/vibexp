import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import type {
  AdminSavedFilterListName,
  AdminSavedFilterPreset,
  AdminSavedFilterPresetInput,
  AdminSavedFilters,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { ApiError } from '@/types/errors'

export type PresetQuery = Record<string, string>

export interface SavedFiltersState {
  status: 'loading' | 'ready' | 'error'
  presets: AdminSavedFilterPreset[]
  version: number
}

/** Turns the current presets into the list to PUT. */
type Transform = (
  presets: readonly AdminSavedFilterPreset[]
) => AdminSavedFilterPresetInput[]

export const CONFLICT_MESSAGE = 'Presets changed in another tab — reloaded'

const INITIAL: SavedFiltersState = {
  status: 'loading',
  presets: [],
  version: 0,
}

const isConflict = (err: unknown) =>
  err instanceof ApiError && err.status === 409

const errorMessage = (err: unknown) =>
  err instanceof Error && err.message ? err.message : 'Could not save presets'

/**
 * The calling admin's saved filter presets for one admin list (#1148), backed
 * by #1147's `GET/PUT /api/v1/admin/saved-filters/{list}`.
 *
 * Every write goes through `commit(transform)`, which PUTs the transformed
 * list with the held `version`. The version is the admin's whole preferences
 * record, so a save in another tab (or a notification-settings change) makes
 * it stale: on a 409 the hook refetches, re-applies the same transform to the
 * fresh presets and PUTs once more. A second failure surfaces a toast and
 * keeps the refetched state.
 *
 * Writes are serialized — a second one while one is in flight is ignored — so
 * two quick clicks cannot race each other into a spurious conflict.
 */
export function useAdminSavedFilters(list: AdminSavedFilterListName) {
  const [state, setState] = useState<SavedFiltersState>(INITIAL)
  const [saving, setSaving] = useState(false)
  // `commit` reads the latest state after awaits, which a closure would not see.
  const stateRef = useRef(state)
  const busyRef = useRef(false)
  const aliveRef = useRef(true)

  const apply = useCallback((next: SavedFiltersState) => {
    stateRef.current = next
    if (aliveRef.current) setState(next)
  }, [])

  const adopt = useCallback(
    (data: AdminSavedFilters): SavedFiltersState => {
      const next: SavedFiltersState = {
        status: 'ready',
        presets: data.presets,
        version: data.version,
      }
      apply(next)
      return next
    },
    [apply]
  )

  const reload = useCallback(async () => {
    apply({ ...stateRef.current, status: 'loading' })
    try {
      adopt(await adminService.getSavedFilters(list))
    } catch {
      apply({ status: 'error', presets: [], version: 0 })
    }
  }, [adopt, apply, list])

  useEffect(() => {
    aliveRef.current = true
    void reload()
    return () => {
      aliveRef.current = false
    }
  }, [reload])

  const commit = useCallback(
    async (transform: Transform): Promise<boolean> => {
      if (busyRef.current) return false
      busyRef.current = true
      setSaving(true)

      const put = (base: SavedFiltersState) =>
        adminService.replaceSavedFilters(list, {
          presets: transform(base.presets).map(({ id, name, query }) =>
            id === undefined ? { name, query } : { id, name, query }
          ),
          version: base.version,
        })

      try {
        try {
          adopt(await put(stateRef.current))
          return true
        } catch (err) {
          if (!isConflict(err)) throw err
        }
        const fresh = adopt(await adminService.getSavedFilters(list))
        try {
          adopt(await put(fresh))
          return true
        } catch (err) {
          if (!isConflict(err)) throw err
          toast.error(CONFLICT_MESSAGE)
          return false
        }
      } catch (err) {
        toast.error(errorMessage(err))
        return false
      } finally {
        busyRef.current = false
        if (aliveRef.current) setSaving(false)
      }
    },
    [adopt, list]
  )

  const save = useCallback(
    (name: string, query: PresetQuery) =>
      // No `id`: the server assigns one to a new preset.
      commit(presets => [...presets, { name: name.trim(), query }]),
    [commit]
  )

  const rename = useCallback(
    (id: string, name: string) =>
      commit(presets =>
        presets.map(preset =>
          preset.id === id ? { ...preset, name: name.trim() } : preset
        )
      ),
    [commit]
  )

  const remove = useCallback(
    (id: string) =>
      commit(presets => presets.filter(preset => preset.id !== id)),
    [commit]
  )

  return { ...state, saving, save, rename, remove, reload }
}
