import type { ArtifactStatus } from '@/services/artifactService'

import {
  ARTIFACT_STATUS_LABEL,
  ARTIFACT_STATUS_OPTIONS,
  artifactStatusTone,
} from '../artifactStatus'

describe('artifactStatus helpers', () => {
  it('maps each status to a distinct badge tone', () => {
    expect(artifactStatusTone('active')).toBe('success')
    expect(artifactStatusTone('draft')).toBe('warning')
    expect(artifactStatusTone('archived')).toBe('neutral')

    const tones = (['active', 'draft', 'archived'] as ArtifactStatus[]).map(
      artifactStatusTone
    )
    expect(new Set(tones).size).toBe(3) // all distinct
  })

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
