import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import type { MetadataResourceType } from '@/services/metadataService'
import { metadataService } from '@/services/metadataService'

const DEFAULT_LIMIT = 100
const DEFAULT_DEBOUNCE_MS = 300

export interface UseMetadataCatalogOptions {
  /** Which resource type's metadata to enumerate. */
  resourceType: MetadataResourceType
  /** Narrow the catalog to a single project. */
  projectId?: string
  /** Debounce window for the value typeahead. */
  debounceMs?: number
  /** Page size for both catalog calls. */
  limit?: number
}

export interface UseMetadataCatalogResult {
  keys: string[]
  keysLoading: boolean
  keysError: string | null
  /** Loads the key catalog. Call it when the popover opens — nothing is fetched before. */
  loadKeys: () => void
  activeKey: string | null
  /** Selects the key whose values to enumerate, resetting the search. */
  selectKey: (key: string | null) => void
  values: string[]
  valuesLoading: boolean
  valuesError: string | null
  /** True when more values exist than were returned — the UI must say so. */
  valuesTruncated: boolean
  valueQuery: string
  setValueQuery: (query: string) => void
}

/**
 * Owns the two metadata-catalog calls behind `MetadataFilter`, so the component
 * itself stays fetch-free and trivially testable.
 *
 * Fetching is lazy: nothing is requested until `loadKeys()` (the popover
 * opening). The value typeahead is debounced and guarded by a monotonic request
 * id, so a slower earlier response can never overwrite a newer one — the same
 * recipe as `useProjectSearch`, which is the closest existing precedent.
 */
export function useMetadataCatalog({
  resourceType,
  projectId,
  debounceMs = DEFAULT_DEBOUNCE_MS,
  limit = DEFAULT_LIMIT,
}: Readonly<UseMetadataCatalogOptions>): UseMetadataCatalogResult {
  const { currentTeam } = useTeam()

  const [keys, setKeys] = useState<string[]>([])
  const [keysLoading, setKeysLoading] = useState(false)
  const [keysError, setKeysError] = useState<string | null>(null)
  const [keysRequested, setKeysRequested] = useState(false)

  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [values, setValues] = useState<string[]>([])
  const [valuesLoading, setValuesLoading] = useState(false)
  const [valuesError, setValuesError] = useState<string | null>(null)
  const [valuesTruncated, setValuesTruncated] = useState(false)
  const [valueQuery, setValueQuery] = useState('')

  // Monotonic request token: a slower earlier fetch must not overwrite the
  // results of a newer one (out-of-order responses while the user types).
  const valuesRequestIdRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const loadKeys = useCallback(() => {
    setKeysRequested(true)
  }, [])

  const selectKey = useCallback((key: string | null) => {
    setActiveKey(key)
    setValueQuery('')
    setValues([])
    setValuesTruncated(false)
    setValuesError(null)
    // Invalidate any value request still in flight for the previous key.
    valuesRequestIdRef.current++
  }, [])

  useEffect(() => {
    if (!keysRequested || !currentTeam) return

    let cancelled = false
    setKeysLoading(true)
    setKeysError(null)

    metadataService
      .listKeys(currentTeam.id, {
        resource_type: resourceType,
        ...(projectId ? { project_id: projectId } : {}),
        limit,
      })
      .then(response => {
        if (cancelled) return
        setKeys(response.keys)
      })
      .catch(() => {
        if (cancelled) return
        setKeysError('Failed to load metadata keys')
        setKeys([])
      })
      .finally(() => {
        if (!cancelled) setKeysLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [keysRequested, currentTeam, resourceType, projectId, limit])

  useEffect(() => {
    if (!activeKey || !currentTeam) return

    if (timerRef.current) clearTimeout(timerRef.current)

    const trimmed = valueQuery.trim()
    const requestId = ++valuesRequestIdRef.current

    timerRef.current = setTimeout(() => {
      setValuesLoading(true)
      setValuesError(null)

      metadataService
        .listValues(currentTeam.id, {
          resource_type: resourceType,
          key: activeKey,
          ...(projectId ? { project_id: projectId } : {}),
          ...(trimmed ? { q: trimmed } : {}),
          limit,
        })
        .then(response => {
          // A newer request started while this one was in flight — discard.
          if (requestId !== valuesRequestIdRef.current) return
          setValues(response.values)
          setValuesTruncated(response.truncated)
        })
        .catch(() => {
          if (requestId !== valuesRequestIdRef.current) return
          setValuesError('Failed to load metadata values')
          setValues([])
          setValuesTruncated(false)
        })
        .finally(() => {
          if (requestId === valuesRequestIdRef.current) setValuesLoading(false)
        })
    }, debounceMs)

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [
    activeKey,
    valueQuery,
    currentTeam,
    resourceType,
    projectId,
    limit,
    debounceMs,
  ])

  return useMemo(
    () => ({
      keys,
      keysLoading,
      keysError,
      loadKeys,
      activeKey,
      selectKey,
      values,
      valuesLoading,
      valuesError,
      valuesTruncated,
      valueQuery,
      setValueQuery,
    }),
    [
      keys,
      keysLoading,
      keysError,
      loadKeys,
      activeKey,
      selectKey,
      values,
      valuesLoading,
      valuesError,
      valuesTruncated,
      valueQuery,
    ]
  )
}
