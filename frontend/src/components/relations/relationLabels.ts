import type {
  RelationResourceType,
  RelationType,
} from '@/services/relationService'

/**
 * Human label for an edge as seen from the queried resource, keyed by
 * (relation_type, direction). `outgoing` = this resource is the subject;
 * `incoming` = this resource is the object, so the label is the read-time
 * inverse (e.g. a stored `supersedes` reads `superseded by` on the target side).
 */
export function relationDirectionLabel(
  relationType: RelationType,
  direction: 'outgoing' | 'incoming'
): string {
  const labels: Record<RelationType, { outgoing: string; incoming: string }> = {
    'governed-by': { outgoing: 'governed by', incoming: 'governs' },
    supersedes: { outgoing: 'supersedes', incoming: 'superseded by' },
    'built-from': { outgoing: 'built from', incoming: 'used to build' },
    'explained-by': { outgoing: 'explained by', incoming: 'explains' },
  }
  return labels[relationType][direction]
}

/**
 * The four relation types a human may create, each paired with the object
 * (target) type the matrix requires — governed-by → blueprint, built-from →
 * prompt, explained-by → memory. `supersedes` requires the same type as the
 * subject, so its target type is resolved per-subject (see targetTypeFor).
 */
export const HUMAN_RELATION_TYPES: {
  value: RelationType
  label: string
  /** Fixed object type, or null when it must match the subject (supersedes). */
  objectType: RelationResourceType | null
}[] = [
  { value: 'governed-by', label: 'Governed by', objectType: 'blueprint' },
  { value: 'built-from', label: 'Built from', objectType: 'prompt' },
  { value: 'explained-by', label: 'Explained by', objectType: 'memory' },
  { value: 'supersedes', label: 'Supersedes', objectType: null },
]

/**
 * Resolves the matrix-allowed target type for a relation type given the subject
 * type (the picker offers only resources of this type, so invalid targets can
 * never be selected).
 */
export function targetTypeFor(
  relationType: RelationType,
  subjectType: RelationResourceType
): RelationResourceType {
  const entry = HUMAN_RELATION_TYPES.find(t => t.value === relationType)
  return entry?.objectType ?? subjectType
}
