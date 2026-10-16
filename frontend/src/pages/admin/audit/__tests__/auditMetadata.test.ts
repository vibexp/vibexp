import { describe, expect, it } from 'vitest'

import {
  DEFAULTS_LABEL,
  describeAuditMetadata,
  formatAuditValue,
} from '../auditMetadata'

describe('describeAuditMetadata', () => {
  it('renders a resource update as old → new per changed field', () => {
    const got = describeAuditMetadata({
      changed_fields: ['title', 'status'],
      title: { old: 'Draft', new: 'Release notes' },
      status: { old: 'draft', new: 'active' },
    })
    expect(got.changes).toEqual([
      { field: 'title', old: 'Draft', new: 'Release notes' },
      { field: 'status', old: 'draft', new: 'active' },
    ])
    expect(got.details).toEqual([])
  })

  it('lists a changed content field without values, since content is never recorded', () => {
    const got = describeAuditMetadata({
      changed_fields: ['body', 'name'],
      name: { old: 'Release notes', new: 'Release notes v2' },
    })
    expect(got.changes).toEqual([
      { field: 'name', old: 'Release notes', new: 'Release notes v2' },
      { field: 'body', valuesWithheld: true },
    ])
    expect(got.details).toEqual([])
  })

  it('renders a move as the project it left and entered, by name', () => {
    const got = describeAuditMetadata({
      from_project_id: 'p-1',
      from_project_name: 'Core',
      to_project_id: 'p-2',
      to_project_name: 'Platform',
    })
    expect(got.changes).toEqual([
      { field: 'project', old: 'Core', new: 'Platform' },
    ])
    expect(got.details).toEqual([])
  })

  it('renders a settings change from its before/after snapshots', () => {
    const got = describeAuditMetadata({
      before: { retention_weeks: 2, capture_ip_ua: true },
      after: { retention_weeks: 4, capture_ip_ua: false },
    })
    expect(got.changes).toEqual([
      { field: 'retention_weeks', old: '2', new: '4' },
      { field: 'capture_ip_ua', old: 'true', new: 'false' },
    ])
  })

  it('names the built-in defaults for a snapshot side that is null', () => {
    const reset = describeAuditMetadata({
      before: { retention_weeks: 4 },
      after: null,
    })
    expect(reset.changes).toEqual([
      { field: 'retention_weeks', old: '4', new: DEFAULTS_LABEL },
    ])
    const firstSave = describeAuditMetadata({
      before: null,
      after: { retention_weeks: 4 },
    })
    expect(firstSave.changes).toEqual([
      { field: 'retention_weeks', old: DEFAULTS_LABEL, new: '4' },
    ])
  })

  it('lists every other key as a detail, structures as compact JSON', () => {
    const got = describeAuditMetadata({
      list: 'users',
      row_count: 12,
      truncated: false,
      filters: { status: 'active' },
    })
    expect(got.changes).toEqual([])
    expect(got.details).toEqual([
      { key: 'list', value: 'users' },
      { key: 'row_count', value: '12' },
      { key: 'truncated', value: 'false' },
      { key: 'filters', value: '{"status":"active"}' },
    ])
  })

  it('keeps an old/new pair with one side missing', () => {
    expect(
      describeAuditMetadata({ title: { new: 'Created' } }).changes
    ).toEqual([{ field: 'title', old: undefined, new: 'Created' }])
  })

  it('is empty for no metadata', () => {
    expect(describeAuditMetadata({})).toEqual({ changes: [], details: [] })
    expect(describeAuditMetadata(undefined)).toEqual({
      changes: [],
      details: [],
    })
  })
})

describe('formatAuditValue', () => {
  it('formats scalars, null and structures', () => {
    expect(formatAuditValue(null)).toBe('—')
    expect(formatAuditValue('x')).toBe('x')
    expect(formatAuditValue(3)).toBe('3')
    expect(formatAuditValue(['a', 'b'])).toBe('["a","b"]')
  })
})
