import { describe, expect, it } from 'vitest'

import {
  CONTENT_WITHHELD,
  DEFAULTS_LABEL,
  describeAuditMetadata,
  formatAuditValue,
  INHERITED_SUFFIX,
  TEAM_SETTINGS_ACTION_LABELS,
  TEAM_SETTINGS_SURFACE_LABELS,
  VALUE_WITHHELD,
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
      { field: 'body', valuesWithheld: true, withheldNote: CONTENT_WITHHELD },
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

  it('renders a team settings update: surface, action, old → new and a secret by name only (#1520)', () => {
    const got = describeAuditMetadata({
      surface: 'model_provider',
      action: 'update',
      provider_id: 'mp-1',
      changed_fields: ['model', 'api_key', 'configuration'],
      model: { old: 'gpt-4o-mini', new: 'gpt-4o' },
    })
    expect(got.changes).toEqual([
      { field: 'model', old: 'gpt-4o-mini', new: 'gpt-4o' },
      { field: 'api_key', valuesWithheld: true, withheldNote: VALUE_WITHHELD },
      {
        field: 'configuration',
        valuesWithheld: true,
        withheldNote: VALUE_WITHHELD,
      },
    ])
    expect(got.details).toEqual([
      { key: 'Settings', value: 'Model provider' },
      { key: 'Action', value: 'Updated' },
      { key: 'Provider', value: 'mp-1' },
    ])
  })

  it('renders a team settings copy with its source team (#1520)', () => {
    const got = describeAuditMetadata({
      provider_id: 'mp-2',
      source_team_id: 'team-src',
      source_provider_id: 'mp-src',
      action: 'copy',
      surface: 'embedding_provider',
      becomes_active: false,
      changed_fields: ['name', 'api_key'],
      name: { new: 'Embed' },
    })
    expect(got.changes).toEqual([
      { field: 'name', old: undefined, new: 'Embed' },
      { field: 'api_key', valuesWithheld: true, withheldNote: VALUE_WITHHELD },
    ])
    expect(got.details.slice(0, 2)).toEqual([
      { key: 'Settings', value: 'Embedding provider' },
      { key: 'Action', value: 'Copied from another team' },
    ])
    expect(got.details).toContainEqual({
      key: 'Source team',
      value: 'team-src',
    })
    expect(got.details).toContainEqual({
      key: 'Source provider',
      value: 'mp-src',
    })
    expect(got.details).toContainEqual({
      key: 'Becomes active',
      value: 'false',
    })
  })

  it('shows an empty team settings detail as none, as a copy that skipped nothing records it (#1520)', () => {
    const got = describeAuditMetadata({
      surface: 'custom_types',
      action: 'copy',
      source_team_id: 'team-src',
      added_slugs: 'runbook',
      skipped_slugs: '',
    })
    expect(got.details).toContainEqual({ key: 'Added', value: 'runbook' })
    expect(got.details).toContainEqual({ key: 'Skipped', value: '—' })
  })

  it('labels the inherited old side of a team\'s first save "(default)" (#1551)', () => {
    const got = describeAuditMetadata({
      surface: 'search',
      action: 'update',
      before_source: 'instance_default',
      changed_fields: ['rank_half_life_days'],
      rank_half_life_days: { old: 90, new: 45 },
    })
    expect(got.changes).toEqual([
      { field: 'rank_half_life_days', old: `90${INHERITED_SUFFIX}`, new: '45' },
    ])
    expect(got.details.map(detail => detail.key)).toEqual([
      'Settings',
      'Action',
    ])
  })

  it('labels the inherited new side of a reset "(default)" (#1551)', () => {
    const got = describeAuditMetadata({
      surface: 'ai_summary',
      action: 'reset',
      after_source: 'instance_default',
      changed_fields: ['top_n'],
      top_n: { old: 8, new: 5 },
    })
    expect(got.changes).toEqual([
      { field: 'top_n', old: '8', new: `5${INHERITED_SUFFIX}` },
    ])
    expect(got.details.map(detail => detail.key)).toEqual([
      'Settings',
      'Action',
    ])
  })

  it('labels every team settings surface and action, and falls back to the raw value', () => {
    // The backend's vocabulary (#1520), exactly.
    expect(Object.keys(TEAM_SETTINGS_SURFACE_LABELS).sort()).toEqual(
      [
        'search',
        'ai_summary',
        'email_provider',
        'model_provider',
        'embedding_provider',
        'freshness',
        'github_app',
        'custom_types',
        'custom_statuses',
      ].sort()
    )
    expect(Object.keys(TEAM_SETTINGS_ACTION_LABELS).sort()).toEqual(
      [
        'create',
        'update',
        'delete',
        'reset',
        'copy',
        'connect',
        'disconnect',
        'rotate',
        'created',
        'renamed',
        'deleted',
      ].sort()
    )
    const got = describeAuditMetadata({ surface: 'future', action: 'zap' })
    expect(got.details).toEqual([
      { key: 'Settings', value: 'future' },
      { key: 'Action', value: 'zap' },
    ])
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
