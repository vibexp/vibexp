import type { AdminInstanceAISummarySettings } from '@/services/adminSettingsService'

import {
  type AISummaryInstanceForm,
  describeAISummaryValues,
  toAISummaryForm,
  toAISummaryUpdate,
  validateAISummaryForm,
} from '../aiSummaryInstanceForm'

const values: AdminInstanceAISummarySettings['values'] = {
  enabled: true,
  top_n: 5,
  style: 'balanced',
  max_output_tokens: 800,
  per_document_chars: 8000,
  total_context_chars: 32000,
  request_timeout_ms: 60000,
}

const limits: AdminInstanceAISummarySettings['limits'] = {
  top_n_min: 1,
  top_n_max: 10,
  max_output_tokens_min: 1,
  max_output_tokens_max: 32768,
  chars_min: 1,
  chars_max: 2147483647,
  request_timeout_ms_min: 1,
  request_timeout_ms_max: 2147483647,
}

const form = (
  patch: Partial<AISummaryInstanceForm> = {}
): AISummaryInstanceForm => ({ ...toAISummaryForm(values), ...patch })

describe('toAISummaryForm / toAISummaryUpdate', () => {
  it('round-trips the values', () => {
    expect(toAISummaryUpdate(toAISummaryForm(values))).toEqual(values)
  })
})

describe('validateAISummaryForm', () => {
  it('accepts valid values, up to the limits', () => {
    expect(validateAISummaryForm(form(), limits)).toEqual({})
    expect(
      validateAISummaryForm(
        form({ top_n: '10', max_output_tokens: '32768' }),
        limits
      )
    ).toEqual({})
  })

  it('checks each numeric field against its own limits', () => {
    expect(
      validateAISummaryForm(
        form({
          top_n: '11',
          max_output_tokens: '0',
          per_document_chars: '1.5',
          request_timeout_ms: '2147483648',
        }),
        limits
      )
    ).toEqual({
      top_n: 'Enter a whole number between 1 and 10.',
      max_output_tokens: 'Enter a whole number between 1 and 32768.',
      per_document_chars: 'Enter a whole number between 1 and 2147483647.',
      request_timeout_ms: 'Enter a whole number between 1 and 2147483647.',
    })
  })

  it('requires the total context to hold one document', () => {
    expect(
      validateAISummaryForm(
        form({ per_document_chars: '9000', total_context_chars: '8999' }),
        limits
      )
    ).toEqual({
      total_context_chars:
        'Must be at least the per-document budget, so one document fits.',
    })
    expect(
      validateAISummaryForm(
        form({ per_document_chars: '9000', total_context_chars: '9000' }),
        limits
      )
    ).toEqual({})
  })

  it('does not stack the budget rule on an already-invalid budget', () => {
    expect(
      validateAISummaryForm(
        form({ per_document_chars: '', total_context_chars: '5' }),
        limits
      )
    ).toEqual({
      per_document_chars: 'Enter a whole number between 1 and 2147483647.',
    })
  })
})

describe('describeAISummaryValues', () => {
  it('summarizes every value', () => {
    expect(describeAISummaryValues(values)).toBe(
      'enabled, 5 results, balanced, 800 tokens, 8000 chars per document, 32000 chars in total, 60000 ms timeout'
    )
  })
})
