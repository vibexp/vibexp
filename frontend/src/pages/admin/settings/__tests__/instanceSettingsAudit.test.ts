import {
  auditActionLabel,
  auditActorLabel,
  diffAuditFields,
  formatAuditValue,
  resettableAuditActionLabel,
} from '../instanceSettingsAudit'

describe('diffAuditFields', () => {
  const fields = [
    ['top_n', 'Results to read'],
    ['enabled', 'Enabled'],
  ] as const

  it('lists only allowlisted fields whose value changed', () => {
    expect(
      diffAuditFields(
        {
          before: { top_n: 5, enabled: true, version: 1 },
          after: { top_n: 7, enabled: true, version: 2 },
        },
        fields
      )
    ).toEqual([
      { key: 'top_n', label: 'Results to read', before: '5', after: '7' },
    ])
  })

  it('lists every set field for a first save and a reset', () => {
    const snapshot = { top_n: 5, enabled: false }
    expect(
      diffAuditFields({ before: null, after: snapshot }, fields).map(
        c => c.before
      )
    ).toEqual(['—', '—'])
    expect(
      diffAuditFields({ before: snapshot, after: null }, fields).map(
        c => c.after
      )
    ).toEqual(['—', '—'])
  })

  it('reads nested keys through a custom reader', () => {
    expect(
      diffAuditFields(
        { before: { a: { b: 1 } }, after: { a: { b: 2 } } },
        [['a.b', 'Nested']],
        (snapshot, key) =>
          (
            snapshot?.[key.split('.')[0]] as Record<string, unknown> | undefined
          )?.[key.split('.')[1]]
      )
    ).toEqual([{ key: 'a.b', label: 'Nested', before: '1', after: '2' }])
  })
})

describe('labels', () => {
  it('names the import as config.yaml, not as a missing actor', () => {
    expect(auditActorLabel({ action: 'import', actor_name: null })).toBe(
      'Imported from config.yaml'
    )
  })

  it('shows the actor, or a deleted user', () => {
    expect(auditActorLabel({ action: 'upsert', actor_name: 'Ada' })).toBe('Ada')
    expect(auditActorLabel({ action: 'delete', actor_name: null })).toBe(
      'Deleted user'
    )
  })

  it('labels a delete of a resettable section as a reset', () => {
    expect(resettableAuditActionLabel('delete')).toBe('Reset to defaults')
    expect(resettableAuditActionLabel('upsert')).toBe('Saved')
    expect(resettableAuditActionLabel('import')).toBe('Imported')
  })

  it('labels each action', () => {
    expect(auditActionLabel('upsert')).toBe('Saved')
    expect(auditActionLabel('delete')).toBe('Removed')
    expect(auditActionLabel('import')).toBe('Imported')
  })

  it('formats values', () => {
    expect(formatAuditValue(null)).toBe('—')
    expect(formatAuditValue('')).toBe('—')
    expect(formatAuditValue(true)).toBe('Yes')
    expect(formatAuditValue(false)).toBe('No')
    expect(formatAuditValue(25)).toBe('25')
    expect(formatAuditValue({ a: 1 })).toBe('{"a":1}')
  })
})
