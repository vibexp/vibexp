import {
  fieldLabel,
  fieldTone,
  type ResourceKindKey,
  resourceRegistry,
  statusFieldOf,
  statusTone,
} from '..'

// Every kind that has a status must map every value it can be in to a tone —
// the whole point of #903 is that a new status value is a one-line descriptor
// change rather than a hunt through per-page ternaries.
const KINDS = Object.keys(resourceRegistry) as ResourceKindKey[]

describe('statusTone', () => {
  it.each(KINDS)('maps every declared status value of %s to a tone', kind => {
    const field = statusFieldOf(resourceRegistry[kind])
    if (!field) return // gallery prompts have no status
    expect(field.statusValues?.length).toBeGreaterThan(0)
    for (const value of field.statusValues ?? []) {
      expect(statusTone(kind, value)).not.toBe('default')
    }
  })

  it('maps each prompt status to the tone the badge renders', () => {
    expect(statusTone('prompt', 'published')).toBe('success')
    expect(statusTone('prompt', 'draft')).toBe('warning')
  })

  it('falls back to neutral for an unknown status', () => {
    expect(statusTone('prompt', 'retired')).toBe('neutral')
    expect(fieldTone(undefined, 'anything')).toBe('neutral')
  })

  it('has no status field for the read-only gallery prompt', () => {
    expect(statusFieldOf(resourceRegistry['gallery-prompt'])).toBeUndefined()
  })
})

describe('fieldLabel', () => {
  it('uses the declared display text when there is one', () => {
    const field = statusFieldOf(resourceRegistry.artifact)
    expect(fieldLabel(field, 'archived')).toBe('Archived')
  })

  it('falls back to the raw value for an unlabelled one', () => {
    // Blueprint statuses are rendered verbatim today; keep it that way.
    expect(
      fieldLabel(statusFieldOf(resourceRegistry.blueprint), 'active')
    ).toBe('active')
    expect(fieldLabel(undefined, 'whatever')).toBe('whatever')
  })
})
