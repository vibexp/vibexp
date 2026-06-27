import type { MemoryStatus } from '@/types'

import {
  MEMORY_STATUS_LABEL,
  MEMORY_STATUS_OPTIONS,
  memoryStatusTone,
} from '../memoryStatus'

describe('memoryStatus helpers', () => {
  it('maps each status to a distinct badge tone', () => {
    expect(memoryStatusTone('active')).toBe('success')
    expect(memoryStatusTone('draft')).toBe('warning')
    expect(memoryStatusTone('archived')).toBe('neutral')

    const tones = (['active', 'draft', 'archived'] as MemoryStatus[]).map(
      memoryStatusTone
    )
    expect(new Set(tones).size).toBe(3) // all distinct
  })

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
