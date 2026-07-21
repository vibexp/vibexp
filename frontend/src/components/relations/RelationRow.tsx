import { Check, Sparkles, X } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { buildResourceUrl } from '@/lib/resourceUrl'
import type { RelatedResource } from '@/services/relationService'

import { relationDirectionLabel } from './relationLabels'

interface RelationRowProps {
  relation: RelatedResource
  /** Caller may confirm a suggested edge (server: resource.update.any). */
  canConfirm: boolean
  /** Caller may delete/dismiss this edge (server: own-vs-any). */
  canDelete: boolean
  /** Accept/Dismiss in flight for this row. */
  busy: boolean
  onConfirm: (relationId: string) => void
  onDismiss: (relationId: string) => void
}

/**
 * One relation in the panel: the direction-aware label, the target resource's
 * title linking to its detail page, an ai-suggested provenance badge, and —
 * only for suggested edges — Accept/Dismiss. Accept confirms; Dismiss deletes.
 */
export function RelationRow({
  relation,
  canConfirm,
  canDelete,
  busy,
  onConfirm,
  onDismiss,
}: Readonly<RelationRowProps>) {
  const label = relationDirectionLabel(
    relation.relation_type,
    relation.direction
  )
  const href = buildResourceUrl({
    type: relation.resource_type,
    id: relation.resource_id,
    slug: relation.slug ?? undefined,
    projectId: relation.project_id ?? undefined,
  })
  const suggested = relation.status === 'suggested'
  const aiSuggested = suggested && relation.origin === 'ai'

  return (
    <div className="flex items-start gap-3 py-3" data-testid="relation-row">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {label}
          </span>
          {aiSuggested && (
            <span
              className="bg-secondary text-secondary-foreground inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium"
              data-testid="relation-suggested-badge"
            >
              <Sparkles aria-hidden="true" className="size-3" />
              AI suggested
            </span>
          )}
        </div>
        <div className="mt-0.5 truncate text-sm font-medium">
          {href ? (
            <Link
              to={href}
              className="hover:underline"
              data-testid="relation-target-link"
            >
              {relation.title || 'Untitled'}
            </Link>
          ) : (
            <span>{relation.title || 'Untitled'}</span>
          )}
        </div>
      </div>

      {suggested && (
        <div className="flex shrink-0 items-center gap-1">
          {canConfirm && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => {
                onConfirm(relation.relation_id)
              }}
              data-testid="relation-accept"
            >
              <Check className="mr-1 size-3.5" />
              Accept
            </Button>
          )}
          {canDelete && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() => {
                onDismiss(relation.relation_id)
              }}
              data-testid="relation-dismiss"
            >
              <X className="mr-1 size-3.5" />
              Dismiss
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
