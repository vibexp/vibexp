import {
  type AISummaryForm,
  clampMaxOutputTokens,
  clampTopN,
  MAX_OUTPUT_TOKENS_MAX,
  TOP_N_MAX,
  validate,
} from './aiSummaryForm'

const form = (patch: Partial<AISummaryForm> = {}): AISummaryForm => ({
  enabled: true,
  model_provider_id: null,
  top_n: '5',
  style: 'balanced',
  max_output_tokens: '800',
  ...patch,
})

describe('hard limits', () => {
  it('are the server bounds (MaxAISummaryTopN / MaxAISummaryOutputTokens)', () => {
    expect(TOP_N_MAX).toBe(10)
    expect(MAX_OUTPUT_TOKENS_MAX).toBe(32768)
  })
})

describe('clampMaxOutputTokens', () => {
  it('keeps an in-range value as typed', () => {
    expect(clampMaxOutputTokens('8000')).toBe('8000')
  })

  it('clamps to the hard limit and to 1', () => {
    expect(clampMaxOutputTokens('40000')).toBe('32768')
    expect(clampMaxOutputTokens('0')).toBe('1')
  })

  it('leaves an empty field alone', () => {
    expect(clampMaxOutputTokens('')).toBe('')
  })
})

describe('clampTopN', () => {
  it('clamps to the hard limit and to 1', () => {
    expect(clampTopN('10')).toBe('10')
    expect(clampTopN('11')).toBe('10')
    expect(clampTopN('0')).toBe('1')
  })
})

describe('validate', () => {
  it('accepts values up to both hard limits', () => {
    expect(
      validate(form({ top_n: '10', max_output_tokens: '32768' }))
    ).toBeNull()
  })

  it.each(['32769', '0', '1.5', ''])(
    'rejects a response length of %j',
    maxOutputTokens => {
      expect(validate(form({ max_output_tokens: maxOutputTokens }))).toBe(
        'Response length must be a whole number between 1 and 32768.'
      )
    }
  )

  it('checks top_n against its own limit, not the token limit', () => {
    expect(validate(form({ top_n: '11' }))).toBe(
      'Results to read must be a whole number between 1 and 10.'
    )
  })
})
