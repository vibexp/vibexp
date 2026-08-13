import type { FreshnessRule } from '@/services/freshnessService'

import {
  ANY_PROJECT,
  describeInterval,
  describeRule,
  emptyRuleForm,
  formToRequest,
  MAX_THRESHOLD_DAYS,
  type RuleFormValues,
  ruleToForm,
  validateRule,
} from '../freshnessOptions'

const rule = (overrides: Partial<FreshnessRule> = {}): FreshnessRule => ({
  id: 'rule-1',
  team_id: 'team-1',
  project_id: null,
  resource_types: ['artifact'],
  mediums: [],
  threshold_days: 90,
  enabled: true,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
  ...overrides,
})

const form = (overrides: Partial<RuleFormValues> = {}): RuleFormValues => ({
  ...emptyRuleForm(),
  resourceTypes: ['artifact'],
  ...overrides,
})

describe('validateRule', () => {
  it('accepts a rule with one resource type and a positive threshold', () => {
    expect(validateRule(form())).toBeNull()
  })

  it('requires at least one resource type', () => {
    expect(validateRule(form({ resourceTypes: [] }))).toMatch(
      /at least one resource type/i
    )
  })

  it('rejects a zero threshold', () => {
    expect(validateRule(form({ thresholdDays: '0' }))).toMatch(
      /at least 1 day/i
    )
  })

  it('rejects a threshold above the server cap', () => {
    expect(
      validateRule(form({ thresholdDays: String(MAX_THRESHOLD_DAYS + 1) }))
    ).toMatch(/at most/i)
  })

  it('accepts the cap itself', () => {
    expect(
      validateRule(form({ thresholdDays: String(MAX_THRESHOLD_DAYS) }))
    ).toBeNull()
  })

  it.each(['1e3', '0x1f', '90.', '9 0', '', 'abc', '-5', '1.5'])(
    'rejects %j, which Number() would accept but the server parser would not',
    value => {
      // The server parses with Go's integer parsing; mirroring the PARSER
      // rather than just the range is what keeps the two in agreement.
      expect(validateRule(form({ thresholdDays: value }))).not.toBeNull()
    }
  )
})

describe('formToRequest', () => {
  it('maps the any-project sentinel to a null project_id', () => {
    expect(formToRequest(form({ projectId: ANY_PROJECT }))).toMatchObject({
      project_id: null,
    })
  })

  it('passes a chosen project through', () => {
    expect(formToRequest(form({ projectId: 'project-7' }))).toMatchObject({
      project_id: 'project-7',
    })
  })

  it('sends every mutable field, because the PUT is a full replacement', () => {
    // An omitted field would reset that dimension of the rule server-side.
    expect(
      Object.keys(formToRequest(form({ mediums: ['web'] }))).sort()
    ).toEqual([
      'enabled',
      'mediums',
      'project_id',
      'resource_types',
      'threshold_days',
    ])
  })

  it('converts the threshold to a number', () => {
    expect(formToRequest(form({ thresholdDays: '45' })).threshold_days).toBe(45)
  })
})

describe('ruleToForm', () => {
  it('round-trips a rule through the form and back', () => {
    const original = rule({
      project_id: 'project-2',
      resource_types: ['prompt', 'memory'],
      mediums: ['cli', 'mcp'],
      threshold_days: 14,
      enabled: false,
    })

    expect(formToRequest(ruleToForm(original))).toEqual({
      project_id: 'project-2',
      resource_types: ['prompt', 'memory'],
      mediums: ['cli', 'mcp'],
      threshold_days: 14,
      enabled: false,
    })
  })

  it('represents a team-wide rule with the any-project sentinel', () => {
    expect(ruleToForm(rule({ project_id: null })).projectId).toBe(ANY_PROJECT)
  })

  it('copies the arrays so editing the form cannot mutate the loaded rule', () => {
    const original = rule({ resource_types: ['artifact'] })
    const values = ruleToForm(original)
    values.resourceTypes.push('prompt')
    expect(original.resource_types).toEqual(['artifact'])
  })
})

describe('describeInterval', () => {
  it.each([
    [3600, 'Hourly'],
    [21600, 'Every 6 hours'],
    [86400, 'Daily'],
    [604800, 'Weekly'],
  ])('names the %d-second preset', (seconds, label) => {
    expect(describeInterval(seconds)).toBe(label)
  })

  it('describes a non-preset interval exactly rather than rounding it', () => {
    expect(describeInterval(259200)).toBe('Every 3 days')
    expect(describeInterval(7200)).toBe('Every 2 hours')
  })

  it('falls back to seconds for an interval that is neither', () => {
    expect(describeInterval(5400)).toBe('Every 5400 seconds')
  })
})

describe('describeRule', () => {
  it('reads as a sentence for a team-wide any-medium rule', () => {
    expect(describeRule(rule())).toBe(
      'Artifacts in any project not accessed via any medium for 90 days'
    )
  })

  it('names the project when the caller resolved it', () => {
    expect(describeRule(rule({ project_id: 'p1' }), 'Marketing')).toBe(
      'Artifacts in Marketing not accessed via any medium for 90 days'
    )
  })

  it('degrades to "a project" when the name is unknown', () => {
    // The rules list must not block on a project lookup it does not have.
    expect(describeRule(rule({ project_id: 'p1' }))).toContain('in a project')
  })

  it('lists multiple types and joins mediums with "or"', () => {
    expect(
      describeRule(
        rule({ resource_types: ['prompt', 'memory'], mediums: ['web', 'cli'] })
      )
    ).toBe(
      'Prompts, Memories in any project not accessed via Web app or CLI for 90 days'
    )
  })

  it('singularises a one-day threshold', () => {
    expect(describeRule(rule({ threshold_days: 1 }))).toContain('for 1 day')
  })
})
