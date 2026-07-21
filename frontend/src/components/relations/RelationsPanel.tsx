import { Plus, Workflow } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { PanelTitle } from '@/components/ui/panel-title'
import { Skeleton } from '@/components/ui/skeleton'
import { useAlerts } from '@/hooks'
import { usePermissions } from '@/hooks/usePermissions'
import { useRelations } from '@/hooks/useRelations'
import type { RelationResourceType } from '@/services/relationService'
import { getErrorMessage } from '@/utils/errorHandling'

import { RelationComposer } from './RelationComposer'
import { RelationRow } from './RelationRow'

interface RelationsPanelProps {
  teamId: string
  resourceType: RelationResourceType
  resourceId: string
}

/**
 * Self-fetching sidebar relations widget for a resource detail page (mirrors the
 * comments panel). Shows the resource's typed neighborhood with direction-aware
 * labels and links to targets; suggested edges carry a provenance badge and
 * Accept/Dismiss (Accept confirms, Dismiss deletes — both optimistic with
 * rollback). A composer adds human edges, matrix-constrained by the picker.
 * Server authorizes every write; the UI gating here is convenience only.
 *
 * Note: RelatedResource carries no created_by, so Dismiss is gated on
 * canDeleteResource(undefined) — admins/owners (resource.delete.any) — since
 * per-edge own-vs-any can't be computed client-side; the server still enforces it.
 */
export function RelationsPanel({
  teamId,
  resourceType,
  resourceId,
}: Readonly<RelationsPanelProps>) {
  const { can, canDeleteResource } = usePermissions()
  const { showError } = useAlerts()
  const state = useRelations(teamId, resourceType, resourceId)

  const [composing, setComposing] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)

  const canAdd = can('resource.create')
  const canConfirm = can('resource.update.any')
  const canDismiss = canDeleteResource(undefined)

  const handleConfirm = async (relationId: string) => {
    setBusyId(relationId)
    try {
      await state.confirmRelation(relationId)
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to confirm relation'),
        'Confirm failed'
      )
    } finally {
      setBusyId(null)
    }
  }

  const handleDismiss = async (relationId: string) => {
    setBusyId(relationId)
    try {
      await state.removeRelation(relationId)
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to dismiss relation'),
        'Dismiss failed'
      )
    } finally {
      setBusyId(null)
    }
  }

  const renderBody = () => {
    if (state.loading) {
      return (
        <div className="space-y-3 px-5 py-4" data-testid="relations-loading">
          {[0, 1, 2].map(i => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      )
    }
    if (state.error) {
      return (
        <div className="flex flex-col items-center gap-2 px-5 py-6 text-center">
          <p className="text-muted-foreground text-sm">
            Couldn&apos;t load relations.
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={state.reload}
          >
            Retry
          </Button>
        </div>
      )
    }
    if (state.relations.length === 0) {
      return (
        <p className="text-muted-foreground px-5 py-6 text-center text-sm">
          No relations yet.
        </p>
      )
    }
    return (
      <div className="divide-border divide-y px-5">
        {state.relations.map(relation => (
          <RelationRow
            key={relation.relation_id}
            relation={relation}
            canConfirm={canConfirm}
            canDelete={canDismiss}
            busy={busyId === relation.relation_id}
            onConfirm={relationId => {
              void handleConfirm(relationId)
            }}
            onDismiss={relationId => {
              void handleDismiss(relationId)
            }}
          />
        ))}
      </div>
    )
  }

  return (
    <div
      className="bg-card text-card-foreground overflow-hidden rounded-lg border shadow-sm"
      data-testid="relations-panel"
    >
      <div className="flex items-center justify-between gap-3 px-5 pt-5 pb-4">
        <div className="flex min-w-0 items-center gap-2.5">
          <Workflow className="text-muted-foreground size-[17px] shrink-0" />
          <PanelTitle>Relations</PanelTitle>
        </div>
        {canAdd && !composing && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setComposing(true)
            }}
            data-testid="relation-add-button"
          >
            <Plus className="mr-1 size-3.5" />
            Add relation
          </Button>
        )}
      </div>

      {canAdd && composing && (
        <div className="px-5 pb-4">
          <RelationComposer
            teamId={teamId}
            subjectType={resourceType}
            subjectId={resourceId}
            onAdd={state.addRelation}
            onSuccess={() => {
              setComposing(false)
            }}
            onCancel={() => {
              setComposing(false)
            }}
          />
        </div>
      )}

      <div className="bg-border h-px" />

      {renderBody()}
    </div>
  )
}
