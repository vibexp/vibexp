import {
  describeResource,
  isCopyEntry,
  surfaceLabel,
} from '../settingsAuditFormat'

const statusEntry = (detail: Record<string, unknown>) => ({
  surface: 'custom_statuses' as const,
  detail,
})

describe('settingsAuditFormat — custom_statuses', () => {
  it('labels the surface', () => {
    expect(surfaceLabel('custom_statuses')).toBe('Statuses')
  })

  it('is the one surface that is not a copy', () => {
    expect(isCopyEntry({ surface: 'custom_statuses' })).toBe(false)
    expect(isCopyEntry({ surface: 'custom_types' })).toBe(true)
    expect(isCopyEntry({ surface: 'model_provider' })).toBe(true)
    expect(isCopyEntry({ surface: 'embedding_provider' })).toBe(true)
  })

  it('falls back to the slug, then to "Unnamed", when the name is missing', () => {
    expect(
      describeResource(statusEntry({ action: 'created', slug: 'in-review' }))
    ).toBe('Created status "in-review"')
    expect(describeResource(statusEntry({ action: 'created' }))).toBe(
      'Created status "Unnamed"'
    )
  })

  it('omits the previous name on a rename that did not record one', () => {
    expect(
      describeResource(
        statusEntry({
          action: 'renamed',
          resource_type: 'prompt',
          name: 'Under review',
        })
      )
    ).toBe('Renamed prompt status to "Under review"')
  })

  it('drops the move clause when the destination is missing', () => {
    expect(
      describeResource(
        statusEntry({
          action: 'deleted',
          resource_type: 'prompt',
          name: 'In review',
          reassigned_count: 4,
        })
      )
    ).toBe('Deleted prompt status "In review"')
  })

  // The enum can grow server-side before this build learns the new value.
  it('stays readable for an action or resource type it does not know', () => {
    expect(
      describeResource(
        statusEntry({
          action: 'archived',
          resource_type: 'feed',
          name: 'In review',
        })
      )
    ).toBe('Changed status "In review"')
  })

  // Only the documented keys are read: a key the server adds later, or a value
  // of the wrong type, never reaches the page.
  it('reads only allowlisted keys and ignores wrongly typed values', () => {
    const text = describeResource(
      statusEntry({
        action: 'deleted',
        resource_type: 'prompt',
        name: 'In review',
        reassigned_count: '12',
        reassigned_to: 'draft',
        internal_note: 'SENTINEL',
      })
    )
    expect(text).toBe('Deleted prompt status "In review"')
    expect(text).not.toContain('SENTINEL')
  })
})
