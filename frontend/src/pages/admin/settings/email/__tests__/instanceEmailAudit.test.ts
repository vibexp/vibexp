import { auditFieldChanges, credentialChange } from '../instanceEmailAudit'

const snapshot = (overrides: Record<string, unknown> = {}) => ({
  provider_type: 'smtp',
  settings: { host: 'smtp.acme.test', port: '587' },
  from_address: 'noreply@acme.test',
  has_credential: true,
  // Bookkeeping the diff must ignore.
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  last_success_at: '2026-09-02T00:00:00Z',
  ...overrides,
})

describe('auditFieldChanges', () => {
  it('lists only the configuration fields that changed', () => {
    expect(
      auditFieldChanges({
        before: snapshot(),
        after: snapshot({
          settings: { host: 'smtp2.acme.test', port: '587' },
          from_name: 'Acme',
          updated_at: '2026-09-03T00:00:00Z',
          last_success_at: null,
          secret: 'unchanged',
        }),
      })
    ).toEqual([
      {
        key: 'settings.host',
        label: 'SMTP host',
        before: 'smtp.acme.test',
        after: 'smtp2.acme.test',
      },
      { key: 'from_name', label: 'Display name', before: '—', after: 'Acme' },
    ])
  })

  it('lists every set field for an import (no before)', () => {
    const changes = auditFieldChanges({ before: null, after: snapshot() })
    expect(changes.map(c => c.label)).toEqual([
      'Provider',
      'SMTP host',
      'SMTP port',
      'From address',
      'Credential stored',
    ])
    expect(changes.every(c => c.before === '—')).toBe(true)
  })

  it('lists every field that was set for a delete (no after)', () => {
    const changes = auditFieldChanges({ before: snapshot(), after: null })
    expect(changes).toHaveLength(5)
    expect(changes.every(c => c.after === '—')).toBe(true)
  })

  it('never surfaces the secret marker as a field', () => {
    const changes = auditFieldChanges({
      before: snapshot(),
      after: snapshot({ secret: 'changed' }),
    })
    expect(changes).toEqual([])
  })
})

describe('credentialChange', () => {
  it.each(['changed', 'unchanged'] as const)('reads the %s marker', marker => {
    expect(credentialChange({ after: snapshot({ secret: marker }) })).toBe(
      marker
    )
  })

  it('is null for an entry without one (import, delete)', () => {
    expect(credentialChange({ after: snapshot() })).toBeNull()
    expect(credentialChange({ after: null })).toBeNull()
  })
})
