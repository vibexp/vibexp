import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { useAlerts } from '@/hooks'
import { artifactService } from '@/services/artifactService'
import { blueprintService } from '@/services/blueprintService'
import { memoryService } from '@/services/memoryService'
import { promptService } from '@/services/promptService'
import type {
  RelationResourceType,
  RelationType,
} from '@/services/relationService'
import { getErrorMessage } from '@/utils/errorHandling'

import { HUMAN_RELATION_TYPES, targetTypeFor } from './relationLabels'

interface TargetOption {
  id: string
  label: string
}

// Lists candidate targets of a given type (matrix-constrained by the caller),
// normalized to {id, label}. Only resources of the allowed type are ever
// offered, so an invalid target cannot be selected.
async function listTargetsForType(
  teamId: string,
  type: RelationResourceType
): Promise<TargetOption[]> {
  switch (type) {
    case 'artifact': {
      const res = await artifactService.getArtifacts(teamId, {})
      return res.artifacts.map(a => ({ id: a.id, label: a.title }))
    }
    case 'blueprint': {
      const res = await blueprintService.getBlueprints(teamId, {})
      return res.blueprints.map(b => ({ id: b.id, label: b.title }))
    }
    case 'prompt': {
      const res = await promptService.getPrompts(teamId, {})
      return res.prompts.map(p => ({ id: p.id, label: p.name }))
    }
    case 'memory': {
      const res = await memoryService.getMemories(teamId, {})
      return res.memories.map(m => ({
        id: m.id,
        label: m.text.slice(0, 60) || 'Untitled memory',
      }))
    }
  }
}

interface RelationComposerProps {
  teamId: string
  subjectType: RelationResourceType
  subjectId: string
  onAdd: (
    relationType: RelationType,
    toType: RelationResourceType,
    toId: string
  ) => Promise<void>
  onSuccess: () => void
  onCancel: () => void
}

const selectClass =
  'border-input bg-background focus-visible:ring-ring h-9 w-full rounded-md border px-3 py-1 text-sm focus-visible:ring-1 focus-visible:outline-none'

/**
 * Inline composer for a human-authored relation: pick the relation type, then a
 * target of the matrix-allowed type (governed-by → blueprint, built-from →
 * prompt, explained-by → memory, supersedes → same type as this resource). The
 * subject resource is excluded so a resource can't relate to itself.
 */
export function RelationComposer({
  teamId,
  subjectType,
  subjectId,
  onAdd,
  onSuccess,
  onCancel,
}: Readonly<RelationComposerProps>) {
  const { showError } = useAlerts()
  const [relationType, setRelationType] = useState<RelationType>('governed-by')
  const [targetId, setTargetId] = useState('')
  const [options, setOptions] = useState<TargetOption[]>([])
  const [loadingOptions, setLoadingOptions] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const targetType = targetTypeFor(relationType, subjectType)
  const loadSeq = useRef(0)

  useEffect(() => {
    const seq = ++loadSeq.current
    setLoadingOptions(true)
    setTargetId('')
    void (async () => {
      try {
        const opts = await listTargetsForType(teamId, targetType)
        if (seq !== loadSeq.current) return // superseded by a newer target type
        // A resource cannot relate to itself.
        setOptions(opts.filter(o => o.id !== subjectId))
      } catch {
        if (seq === loadSeq.current) setOptions([])
      } finally {
        if (seq === loadSeq.current) setLoadingOptions(false)
      }
    })()
  }, [teamId, targetType, subjectId])

  const handleSubmit = useCallback(async () => {
    if (!targetId) return
    setSubmitting(true)
    try {
      await onAdd(relationType, targetType, targetId)
      onSuccess()
    } catch (err) {
      showError(getErrorMessage(err, 'Failed to add relation'), 'Add failed')
    } finally {
      setSubmitting(false)
    }
  }, [targetId, relationType, targetType, onAdd, onSuccess, showError])

  return (
    <div className="space-y-3" data-testid="relation-composer">
      <div className="space-y-1">
        <label
          className="text-muted-foreground text-xs font-medium"
          htmlFor="relation-type"
        >
          Relation
        </label>
        <select
          id="relation-type"
          className={selectClass}
          value={relationType}
          onChange={e => {
            setRelationType(e.target.value as RelationType)
          }}
          data-testid="relation-type-select"
        >
          {HUMAN_RELATION_TYPES.map(t => (
            <option key={t.value} value={t.value}>
              {t.label}
            </option>
          ))}
        </select>
      </div>

      <div className="space-y-1">
        <label
          className="text-muted-foreground text-xs font-medium"
          htmlFor="relation-target"
        >
          Target {targetType}
        </label>
        <select
          id="relation-target"
          className={selectClass}
          value={targetId}
          disabled={loadingOptions}
          onChange={e => {
            setTargetId(e.target.value)
          }}
          data-testid="relation-target-select"
        >
          <option value="">
            {loadingOptions ? 'Loading…' : `Select a ${targetType}…`}
          </option>
          {options.map(o => (
            <option key={o.id} value={o.id}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={!targetId || submitting}
          onClick={() => {
            void handleSubmit()
          }}
          data-testid="relation-add-submit"
        >
          Add relation
        </Button>
      </div>
    </div>
  )
}
