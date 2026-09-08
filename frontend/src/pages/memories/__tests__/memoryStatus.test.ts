import { MEMORY_STATUS_LABEL, MEMORY_STATUS_OPTIONS } from '../memoryStatus'

// Tones are the descriptor's, not this module's (#903/#907) — they are pinned
// by the resource pattern's own tests.
describe('memoryStatus helpers', () => {
  it('labels every status', () => {
    expect(MEMORY_STATUS_LABEL).toEqual({
      active: 'Active',
      draft: 'Draft',
      archived: 'Archived',
    })
  })

  it('exposes select options in display order', () => {
    expect(MEMORY_STATUS_OPTIONS.map(o => o.value)).toEqual([
      'active',
      'draft',
      'archived',
    ])
  })
})
