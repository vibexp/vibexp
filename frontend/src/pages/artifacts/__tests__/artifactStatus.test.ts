import { ARTIFACT_STATUS_OPTIONS } from '../artifactStatus'

// Tones and labels are the descriptor's, not this module's (#903/#907) — they
// are pinned by the resource pattern's own tests. What is left here is the
// Select options the form and the filter bar consume.
describe('artifactStatus helpers', () => {
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
