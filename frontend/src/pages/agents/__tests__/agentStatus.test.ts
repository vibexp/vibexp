import { fieldLabel, fieldTone } from '@/components/patterns/resource'

import { agentStatusField } from '../agentStatus'

/*
 * `agentStatusField` is hand-written rather than validated by `defineResource`
 * (an agent is not a registered resource kind), so the invariants that
 * validator enforces are pinned here instead: a tone or a label for a value the
 * field does not declare would otherwise compile and degrade silently to
 * `neutral` / the raw string.
 */
describe('agentStatusField', () => {
  const values = agentStatusField.statusValues ?? []

  it('declares every status the API can return', () => {
    expect(values).toEqual(['active', 'paused', 'error'])
  })

  it('gives every declared value a tone and a label of its own', () => {
    const tones = agentStatusField.tone ?? {}
    for (const value of values) {
      expect(fieldTone(agentStatusField, value)).toBe(
        new Map(Object.entries(tones)).get(value)
      )
      expect(fieldLabel(agentStatusField, value)).not.toBe(value)
    }
  })

  it('falls back to neutral for a status it has never heard of', () => {
    expect(fieldTone(agentStatusField, 'quarantined')).toBe('neutral')
  })

  it('declares no tone or label for a value it does not list', () => {
    const declared = new Set<string>(values)
    const keyed = [
      ...Object.keys(agentStatusField.tone ?? {}),
      ...Object.keys(agentStatusField.valueLabels ?? {}),
    ]
    expect(keyed.filter(key => !declared.has(key))).toEqual([])
  })
})
