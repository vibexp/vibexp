import { describe, expect, it } from 'vitest'

import {
  AUDIT_FILTER_DEFAULTS,
  AUDIT_PAGE_SIZE,
  auditExportParams,
  buildAuditListParams,
  defaultAuditRange,
  effectiveAuditRange,
  joinList,
  selectedOperations,
  splitList,
} from '../auditListParams'

const NOW = new Date(2026, 9, 16, 15, 30) // 16 Oct 2026, local time

describe('the default range', () => {
  it('is the last seven local days, today included', () => {
    const { from, to } = defaultAuditRange(NOW)
    expect(from).toEqual(new Date(2026, 9, 10))
    expect(to).toEqual(new Date(2026, 9, 16))
  })

  it('applies only when the URL carries no range', () => {
    expect(effectiveAuditRange({}, NOW)).toEqual({
      range: defaultAuditRange(NOW),
      isDefault: true,
    })
    const explicit = { from: new Date(2026, 0, 1), to: new Date(2026, 0, 31) }
    expect(effectiveAuditRange(explicit, NOW)).toEqual({
      range: explicit,
      isDefault: false,
    })
  })

  it('keeps a one-sided URL range one-sided', () => {
    const fromOnly = { from: new Date(2026, 0, 1) }
    expect(effectiveAuditRange(fromOnly, NOW)).toEqual({
      range: fromOnly,
      isDefault: false,
    })
  })
})

describe('buildAuditListParams', () => {
  const range = defaultAuditRange(NOW)

  it('sends only the page size and the range when nothing is filtered', () => {
    const params = buildAuditListParams(AUDIT_FILTER_DEFAULTS, {
      range,
      limit: AUDIT_PAGE_SIZE,
    })
    expect(params).toEqual({
      limit: AUDIT_PAGE_SIZE,
      from: new Date(2026, 9, 10).toISOString(),
      to: new Date(2026, 9, 16, 23, 59, 59, 999).toISOString(),
    })
  })

  it('maps every URL filter onto its API param', () => {
    const params = buildAuditListParams(
      {
        ...AUDIT_FILTER_DEFAULTS,
        search: '  ada@example.com ',
        actor_user_id: 'u-1',
        team_id: 't-1',
        project_id: 'p-1',
        operation: 'prompt.update,authz.denied',
        resource_type: 'prompt',
        source: 'mcp,cli',
        outcome: 'denied',
      },
      { range, limit: 25, cursor: 'abc' }
    )
    expect(params).toMatchObject({
      limit: 25,
      cursor: 'abc',
      q: 'ada@example.com',
      actor_user_id: 'u-1',
      team_id: 't-1',
      project_id: 'p-1',
      operation: ['prompt.update', 'authz.denied'],
      resource_type: 'prompt',
      source: ['mcp', 'cli'],
      outcome: ['denied'],
    })
  })

  it('drops enum values a hand-edited URL made up, rather than sending a 400', () => {
    const params = buildAuditListParams(
      {
        ...AUDIT_FILTER_DEFAULTS,
        operation: 'prompt.update,widget.exploded',
        source: 'fax',
        outcome: 'maybe',
      },
      { range, limit: AUDIT_PAGE_SIZE }
    )
    expect(params.operation).toEqual(['prompt.update'])
    expect(params).not.toHaveProperty('source')
    expect(params).not.toHaveProperty('outcome')
  })

  it('sends no q for blank search text', () => {
    const params = buildAuditListParams(
      { ...AUDIT_FILTER_DEFAULTS, search: '   ' },
      { range, limit: AUDIT_PAGE_SIZE }
    )
    expect(params).not.toHaveProperty('q')
  })

  it('exports the same filters without the cursor or page size', () => {
    const params = buildAuditListParams(
      { ...AUDIT_FILTER_DEFAULTS, team_id: 't-1', outcome: 'failure' },
      { range, limit: AUDIT_PAGE_SIZE, cursor: 'next' }
    )
    const exported = auditExportParams(params)
    expect(exported).not.toHaveProperty('cursor')
    expect(exported).not.toHaveProperty('limit')
    expect(exported).toMatchObject({
      team_id: 't-1',
      outcome: ['failure'],
      from: params.from,
      to: params.to,
    })
  })
})

describe('list values in the URL', () => {
  it('round-trips through split and join, deduplicating known operations', () => {
    expect(splitList('a, b,,c')).toEqual(['a', 'b', 'c'])
    expect(splitList('')).toEqual([])
    expect(joinList(['a', 'b'])).toBe('a,b')
    expect(selectedOperations('auth.login,auth.login')).toEqual(['auth.login'])
  })
})
