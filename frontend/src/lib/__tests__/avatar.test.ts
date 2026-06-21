import { assistantColor, assistantInitial } from '../avatar'

const CHART_TOKENS = [
  'bg-chart-1',
  'bg-chart-2',
  'bg-chart-3',
  'bg-chart-4',
  'bg-chart-5',
]

describe('assistantColor', () => {
  it('returns an on-token chart palette class', () => {
    expect(CHART_TOKENS).toContain(assistantColor('VibeXP'))
  })

  it('is deterministic — the same name always maps to the same colour', () => {
    expect(assistantColor('Claude')).toBe(assistantColor('Claude'))
    expect(assistantColor('Gemini')).toBe(assistantColor('Gemini'))
  })

  it('only ever returns design-system chart tokens', () => {
    const names = ['Claude', 'Gemini', 'Codex', 'Mistral', 'a', 'zzz', 'Δ']
    for (const name of names) {
      expect(CHART_TOKENS).toContain(assistantColor(name))
    }
  })

  it('handles an empty name without throwing', () => {
    expect(CHART_TOKENS).toContain(assistantColor(''))
  })
})

describe('assistantInitial', () => {
  it('returns the uppercased first character', () => {
    expect(assistantInitial('claude')).toBe('C')
    expect(assistantInitial('Gemini')).toBe('G')
  })

  it('falls back to "?" for an empty name', () => {
    expect(assistantInitial('')).toBe('?')
  })
})
