import { MEMORY_STATUS_OPTIONS } from '../memoryStatus'

// Tones and labels are the descriptor's, not this module's (#903/#907) — they
// are pinned by the resource pattern's own tests. What is left here is the
// Select options the form and the filter bar consume.
describe('memoryStatus helpers', () => {
  it('exposes select options in display order', () => {
    expect(MEMORY_STATUS_OPTIONS.map(o => o.value)).toEqual([
      'active',
      'draft',
      'archived',
    ])
  })
})
