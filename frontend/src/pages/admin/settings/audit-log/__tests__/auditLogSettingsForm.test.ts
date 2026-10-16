import type { AdminAuditLogSettings } from '@/services/adminSettingsService'

import {
  type AuditLogSettingsForm,
  describeAuditLogReset,
  describeAuditLogValues,
  retentionLoweredWarning,
  toAuditLogEnvelope,
  toAuditLogForm,
  toAuditLogUpdate,
  validateAuditLogForm,
} from '../auditLogSettingsForm'

const values: AdminAuditLogSettings['values'] = {
  retention_weeks: 8,
  capture_ip_ua: true,
}

const limits: AdminAuditLogSettings['limits'] = {
  retention_weeks_min: 1,
  retention_weeks_max: 52,
}

const form = (
  patch: Partial<AuditLogSettingsForm> = {}
): AuditLogSettingsForm => ({ ...toAuditLogForm(values), ...patch })

describe('toAuditLogForm / toAuditLogUpdate', () => {
  it('round-trips the values', () => {
    expect(toAuditLogUpdate(toAuditLogForm(values))).toEqual(values)
  })
})

describe('toAuditLogEnvelope', () => {
  it('keeps the response and adds a zero override count', () => {
    const response: AdminAuditLogSettings = {
      values,
      built_in_defaults: { retention_weeks: 2, capture_ip_ua: true },
      limits,
      source: 'instance',
      updated_at: '2026-10-16T10:00:00Z',
      updated_by_user_id: 'u1',
      updated_by_name: 'Ada Admin',
      version: 3,
    }
    expect(toAuditLogEnvelope(response)).toEqual({
      values,
      built_in_defaults: response.built_in_defaults,
      limits,
      source: 'instance',
      teams_with_override: 0,
      updated_at: response.updated_at,
      updated_by_name: 'Ada Admin',
      version: 3,
    })
  })
})

describe('validateAuditLogForm', () => {
  it('accepts the bounds', () => {
    expect(
      validateAuditLogForm(form({ retention_weeks: '1' }), limits)
    ).toEqual({})
    expect(
      validateAuditLogForm(form({ retention_weeks: '52' }), limits)
    ).toEqual({})
  })

  it.each(['0', '53', '2.5', '', 'abc', '-1'])('rejects retention %j', raw => {
    expect(
      validateAuditLogForm(form({ retention_weeks: raw }), limits)
    ).toEqual({
      retention_weeks: 'Enter a whole number between 1 and 52.',
    })
  })

  it('takes the bounds from the limits', () => {
    expect(
      validateAuditLogForm(form({ retention_weeks: '20' }), {
        retention_weeks_min: 1,
        retention_weeks_max: 12,
      }).retention_weeks
    ).toBe('Enter a whole number between 1 and 12.')
  })
})

describe('retentionLoweredWarning', () => {
  it('warns below the saved retention', () => {
    expect(
      retentionLoweredWarning(form({ retention_weeks: '4' }), values)
    ).toBe(
      'Audit events older than 4 weeks will be deleted at the next daily purge. This cannot be undone.'
    )
  })

  it('uses the singular for one week', () => {
    expect(
      retentionLoweredWarning(form({ retention_weeks: '1' }), values)
    ).toMatch(/older than 1 week will/)
  })

  it.each(['8', '9', '52', '', '0', '2.5', 'abc'])(
    'stays quiet for %j',
    raw => {
      expect(
        retentionLoweredWarning(form({ retention_weeks: raw }), values)
      ).toBeNull()
    }
  )
})

describe('describeAuditLogValues', () => {
  it('summarizes the values', () => {
    expect(
      describeAuditLogValues({ retention_weeks: 2, capture_ip_ua: true })
    ).toBe('retention 2 weeks, IP address and user agent capture on')
    expect(
      describeAuditLogValues({ retention_weeks: 1, capture_ip_ua: false })
    ).toBe('retention 1 week, IP address and user agent capture off')
  })
})

describe('describeAuditLogReset', () => {
  const builtIn = { retention_weeks: 2, capture_ip_ua: true }

  it('adds the purge when the defaults keep events for less time', () => {
    expect(describeAuditLogReset(builtIn, values)).toBe(
      'retention 2 weeks, IP address and user agent capture on. Audit events older than 2 weeks will be deleted at the next daily purge'
    )
  })

  it('is the plain summary otherwise', () => {
    expect(
      describeAuditLogReset(builtIn, { ...values, retention_weeks: 2 })
    ).toBe('retention 2 weeks, IP address and user agent capture on')
  })
})
