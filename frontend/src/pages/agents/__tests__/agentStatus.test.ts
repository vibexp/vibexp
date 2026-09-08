import { fieldLabel, fieldTone } from '@/components/patterns/resource'

import { agentStatusField } from '../agentStatus'

/*
 * `agentStatusField` now travels on the `agent` descriptor (#918), so
 * `defineResource` validates the invariants a status field shares with every
 * other kind. These stay because they pin the ones it does not: that the field
 * still reaches the agents list through `pages/agents/agentStatus`, and that a
 * tone or a label for a value the field does not declare would degrade silently
 * to `neutral` / the raw string rather than fail.
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
