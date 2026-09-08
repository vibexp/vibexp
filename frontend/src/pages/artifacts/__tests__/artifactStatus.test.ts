import {
  ARTIFACT_STATUS_LABEL,
  ARTIFACT_STATUS_OPTIONS,
} from '../artifactStatus'

// Tones are the descriptor's, not this module's (#903/#907) — they are pinned
// by the resource pattern's own tests.
describe('artifactStatus helpers', () => {
  it('labels every status', () => {
    expect(ARTIFACT_STATUS_LABEL).toEqual({
      active: 'Active',
      draft: 'Draft',
      archived: 'Archived',
    })
  })

  it('exposes select options in display order without the retired "expired"', () => {
    expect(ARTIFACT_STATUS_OPTIONS.map(o => o.value)).toEqual([
      'active',
      'draft',
      'archived',
    ])
    expect(
      ARTIFACT_STATUS_OPTIONS.some(o => (o.value as string) === 'expired')
    ).toBe(false)
  })
})
