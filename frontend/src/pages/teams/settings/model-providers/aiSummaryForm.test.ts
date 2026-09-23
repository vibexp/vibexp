import {
  type AISummaryForm,
  clampMaxOutputTokens,
  validate,
} from './aiSummaryForm'

const limits = { maxTopN: 10, maxOutputTokens: 4096 }

const form = (patch: Partial<AISummaryForm> = {}): AISummaryForm => ({
  enabled: true,
  model_provider_id: null,
  top_n: '5',
  style: 'balanced',
  max_output_tokens: '800',
  ...patch,
})

describe('clampMaxOutputTokens', () => {
  it('keeps an in-range value as typed', () => {
    expect(clampMaxOutputTokens('800', 4096)).toBe('800')
  })

  it('clamps to the ceiling and to 1', () => {
    expect(clampMaxOutputTokens('5000', 4096)).toBe('4096')
    expect(clampMaxOutputTokens('0', 4096)).toBe('1')
  })

  it('leaves an empty field alone', () => {
    expect(clampMaxOutputTokens('', 4096)).toBe('')
  })
})

describe('validate', () => {
  it('accepts values inside both ceilings', () => {
    expect(validate(form({ max_output_tokens: '4096' }), limits)).toBeNull()
  })

  it.each(['4097', '0', '1.5', ''])(
    'rejects a response length of %j',
    maxOutputTokens => {
      expect(
        validate(form({ max_output_tokens: maxOutputTokens }), limits)
      ).toBe('Response length must be a whole number between 1 and 4096.')
    }
  )

  it('checks top_n against maxTopN, not the token ceiling', () => {
    expect(validate(form({ top_n: '11' }), limits)).toBe(
      'Results to read must be a whole number between 1 and 10.'
    )
  })
})
