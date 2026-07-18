import { useCallback, useEffect, useState } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import type { Type } from '@/services/typeService'
import { typeService } from '@/services/typeService'

interface UseTypesResult {
  types: Type[]
  isLoading: boolean
  reload: () => void
}

// useTypes fetches the team's types for a resource (system defaults + custom),
// used to render artifact type dropdowns. Reloads when the current team changes.
export function useTypes(resourceType: string): UseTypesResult {
  const { currentTeam } = useTeam()
  const [types, setTypes] = useState<Type[]>([])
  const [isLoading, setIsLoading] = useState(true)

  const teamId = currentTeam?.id

  const load = useCallback(() => {
    if (!teamId) return undefined
    let cancelled = false
    setIsLoading(true)
    typeService
      .getTypes(teamId, resourceType)
      .then(result => {
        if (!cancelled) setTypes(result)
      })
      .catch(() => {
        if (!cancelled) setTypes([])
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [teamId, resourceType])

  useEffect(() => load(), [load])

  return {
    types,
    isLoading,
    reload: () => {
      load()
    },
  }
}
