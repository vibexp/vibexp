import { useCallback, useEffect, useRef, useState } from 'react'

import type {
  RelatedResource,
  RelationResourceType,
  RelationType,
} from '@/services/relationService'
import { relationService } from '@/services/relationService'

// The panel shows a resource's whole depth-1 neighborhood; the server caps the
// list, so a single page covers it.
const RELATIONS_PAGE_LIMIT = 100

export interface UseRelationsResult {
  relations: RelatedResource[]
  loading: boolean
  error: boolean
  reload: () => void
  /** Create a human edge from this resource to the picked target, then reload. */
  addRelation: (
    relationType: RelationType,
    toType: RelationResourceType,
    toId: string
  ) => Promise<void>
  /** Optimistically flip a suggested edge to confirmed (rolls back on error). */
  confirmRelation: (relationId: string) => Promise<void>
  /** Optimistically remove an edge (rolls back on error). */
  removeRelation: (relationId: string) => Promise<void>
}

/**
 * Self-contained data layer for a resource's typed relations (useComments-
 * shaped): a both-directions list plus optimistic confirm/remove. Adds reload
 * after a create so the row carries the server-hydrated title and direction. A
 * seq ref guards against a stale earlier load overwriting the current resource
 * (the resourceId prop can change in place across detail-page navigations).
 */
export function useRelations(
  teamId: string,
  resourceType: RelationResourceType,
  resourceId: string
): UseRelationsResult {
  const [relations, setRelations] = useState<RelatedResource[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const seqRef = useRef(0)
  // Mirrors `relations` so an optimistic mutation can capture the pre-mutation
  // list for rollback without depending on the setState updater having run.
  const relationsRef = useRef<RelatedResource[]>([])
  useEffect(() => {
    relationsRef.current = relations
  }, [relations])

  const load = useCallback(() => {
    const seq = ++seqRef.current
    setLoading(true)
    setError(false)
    void (async () => {
      try {
        const res = await relationService.list(
          teamId,
          resourceType,
          resourceId,
          1,
          RELATIONS_PAGE_LIMIT
        )
        if (seq !== seqRef.current) return
        setRelations(res.relations)
      } catch {
        if (seq !== seqRef.current) return
        setRelations([])
        setError(true)
      } finally {
        if (seq === seqRef.current) setLoading(false)
      }
    })()
  }, [teamId, resourceType, resourceId])

  useEffect(() => {
    load()
  }, [load])

  const addRelation = useCallback(
    async (
      relationType: RelationType,
      toType: RelationResourceType,
      toId: string
    ) => {
      await relationService.create(teamId, {
        from_type: resourceType,
        from_id: resourceId,
        to_type: toType,
        to_id: toId,
        relation_type: relationType,
        origin: 'human',
      })
      load()
    },
    [teamId, resourceType, resourceId, load]
  )

  const confirmRelation = useCallback(
    async (relationId: string) => {
      const prev = relationsRef.current
      setRelations(rs =>
        rs.map(r =>
          r.relation_id === relationId ? { ...r, status: 'confirmed' } : r
        )
      )
      try {
        await relationService.confirm(teamId, relationId)
      } catch (err) {
        setRelations(prev)
        throw err
      }
    },
    [teamId]
  )

  const removeRelation = useCallback(
    async (relationId: string) => {
      const prev = relationsRef.current
      setRelations(rs => rs.filter(r => r.relation_id !== relationId))
      try {
        await relationService.remove(teamId, relationId)
      } catch (err) {
        setRelations(prev)
        throw err
      }
    },
    [teamId]
  )

  return {
    relations,
    loading,
    error,
    reload: load,
    addRelation,
    confirmRelation,
    removeRelation,
  }
}
