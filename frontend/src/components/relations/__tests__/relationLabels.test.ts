import {
  HUMAN_RELATION_TYPES,
  relationDirectionLabel,
  targetTypeFor,
} from '@/components/relations/relationLabels'

test('direction labels invert on the incoming side', () => {
  expect(relationDirectionLabel('supersedes', 'outgoing')).toBe('supersedes')
  expect(relationDirectionLabel('supersedes', 'incoming')).toBe('superseded by')
  expect(relationDirectionLabel('governed-by', 'outgoing')).toBe('governed by')
  expect(relationDirectionLabel('governed-by', 'incoming')).toBe('governs')
  expect(relationDirectionLabel('built-from', 'outgoing')).toBe('built from')
  expect(relationDirectionLabel('explained-by', 'incoming')).toBe('explains')
})

test('targetTypeFor enforces the constraint matrix', () => {
  // Object type is fixed per relation type...
  expect(targetTypeFor('governed-by', 'artifact')).toBe('blueprint')
  expect(targetTypeFor('built-from', 'memory')).toBe('prompt')
  expect(targetTypeFor('explained-by', 'artifact')).toBe('memory')
  // ...except supersedes, which requires the same type as the subject.
  expect(targetTypeFor('supersedes', 'prompt')).toBe('prompt')
  expect(targetTypeFor('supersedes', 'blueprint')).toBe('blueprint')
})

test('every human relation type carries a matrix object type or same-type marker', () => {
  expect(HUMAN_RELATION_TYPES).toHaveLength(4)
  const supersedes = HUMAN_RELATION_TYPES.find(t => t.value === 'supersedes')
  expect(supersedes?.objectType).toBeNull()
  const governedBy = HUMAN_RELATION_TYPES.find(t => t.value === 'governed-by')
  expect(governedBy?.objectType).toBe('blueprint')
})
