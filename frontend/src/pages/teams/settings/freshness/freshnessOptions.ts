import type {
  FreshnessRule,
  FreshnessRuleMedium,
  FreshnessRuleResourceType,
  UpdateFreshnessRuleRequest,
} from '@/services/freshnessService'

/**
 * Options, formatting and validation for the freshness rule editor.
 *
 * A sibling data module rather than consts inside the page, for the same reason
 * as `team-settings-cards.ts` and `searchRankingPresets.ts`: a `.tsx` exporting
 * both a component and a value trips `react-refresh/only-export-components`,
 * and keeping the pure logic here makes it testable without rendering anything.
 */

export const RESOURCE_TYPE_OPTIONS: {
  value: FreshnessRuleResourceType
  label: string
}[] = [
  { value: 'artifact', label: 'Artifacts' },
  { value: 'prompt', label: 'Prompts' },
  { value: 'blueprint', label: 'Blueprints' },
  { value: 'memory', label: 'Memories' },
]

export const MEDIUM_OPTIONS: {
  value: FreshnessRuleMedium
  label: string
}[] = [
  { value: 'web', label: 'Web app' },
  { value: 'cli', label: 'CLI' },
  { value: 'mcp', label: 'MCP tools' },
]

/**
 * Evaluation-interval presets.
 *
 * The server enforces a one-hour floor (and a 365-day ceiling), so `hourly` is
 * the fastest offer-able cadence rather than an arbitrary choice. A team whose
 * stored interval matches no preset — set through the API directly — keeps its
 * value and is described exactly, so opening this page never silently rounds it.
 */
export const INTERVAL_PRESETS: { seconds: number; label: string }[] = [
  { seconds: 3600, label: 'Hourly' },
  { seconds: 21600, label: 'Every 6 hours' },
  { seconds: 86400, label: 'Daily' },
  { seconds: 604800, label: 'Weekly' },
]

/**
 * Seeds the interval state before the first load resolves; the real value
 * always comes from the server. The interval's own bounds (3600 / 31536000) are
 * not mirrored here — every offered option is a preset or a value the server
 * already accepted, so there is nothing for a client-side bound to catch.
 */
export const DEFAULT_INTERVAL_SECONDS = 86400

/** The server's threshold cap, mirrored so a bad value is caught before the call. */
export const MAX_THRESHOLD_DAYS = 36500

/** The rule editor's form state — strings, because the inputs produce strings. */
export interface RuleFormValues {
  projectId: string
  resourceTypes: FreshnessRuleResourceType[]
  mediums: FreshnessRuleMedium[]
  thresholdDays: string
  enabled: boolean
}

/** `''` is the form's "any project" sentinel; the wire format uses `null`. */
export const ANY_PROJECT = ''

export function emptyRuleForm(): RuleFormValues {
  return {
    projectId: ANY_PROJECT,
    resourceTypes: [],
    mediums: [],
    thresholdDays: '90',
    enabled: true,
  }
}

export function ruleToForm(rule: FreshnessRule): RuleFormValues {
  return {
    projectId: rule.project_id ?? ANY_PROJECT,
    resourceTypes: [...rule.resource_types],
    mediums: [...rule.mediums],
    thresholdDays: String(rule.threshold_days),
    enabled: rule.enabled,
  }
}

/**
 * Converts form state to the wire shape.
 *
 * Typed as the UPDATE request — the one whose fields are all required — because
 * the PUT is a complete replacement and an omitted field would reset that
 * dimension of the rule. That shape is also assignable to the create request,
 * whose `project_id` and `mediums` are merely optional, so one converter serves
 * both calls without either being able to drop a field.
 */
export function formToRequest(
  form: RuleFormValues
): UpdateFreshnessRuleRequest {
  return {
    project_id: form.projectId === ANY_PROJECT ? null : form.projectId,
    resource_types: form.resourceTypes,
    mediums: form.mediums,
    threshold_days: Number(form.thresholdDays),
    enabled: form.enabled,
  }
}

/**
 * Mirrors the server's validation so an invalid rule is caught before the
 * round trip. It deliberately mirrors the server's PARSER too: `Number()`
 * accepts `1e3`, `0x1f` and `90.`, which Go's integer parsing does not, so the
 * digits-only test is what keeps the two in agreement.
 *
 * Returns `null` when the form is valid.
 */
export function validateRule(form: RuleFormValues): string | null {
  if (form.resourceTypes.length === 0) {
    return 'Select at least one resource type.'
  }
  if (!/^\d+$/.test(form.thresholdDays.trim())) {
    return 'Threshold must be a whole number of days.'
  }
  const days = Number(form.thresholdDays)
  if (days <= 0) {
    return 'Threshold must be at least 1 day.'
  }
  if (days > MAX_THRESHOLD_DAYS) {
    return `Threshold must be at most ${String(MAX_THRESHOLD_DAYS)} days.`
  }
  return null
}

/** Formats an interval as the matching preset label, or as a plain duration. */
export function describeInterval(seconds: number): string {
  const preset = INTERVAL_PRESETS.find(p => p.seconds === seconds)
  if (preset) return preset.label
  if (seconds % 86400 === 0) {
    const days = seconds / 86400
    return `Every ${String(days)} day${days === 1 ? '' : 's'}`
  }
  if (seconds % 3600 === 0) {
    const hours = seconds / 3600
    return `Every ${String(hours)} hour${hours === 1 ? '' : 's'}`
  }
  return `Every ${String(seconds)} seconds`
}

/**
 * Renders a rule as the sentence the rules table shows, e.g.
 * "Artifacts in Marketing not accessed via the CLI for 90 days".
 *
 * `projectName` is resolved by the caller from the projects it already loaded;
 * an unknown id degrades to "a project" rather than blocking on a lookup.
 */
export function describeRule(
  rule: FreshnessRule,
  projectName?: string
): string {
  const types = rule.resource_types
    .map(t => RESOURCE_TYPE_OPTIONS.find(o => o.value === t)?.label ?? t)
    .join(', ')

  const scope =
    rule.project_id === null
      ? 'in any project'
      : `in ${projectName ?? 'a project'}`

  const mediums =
    rule.mediums.length === 0
      ? 'via any medium'
      : `via ${rule.mediums
          .map(m => MEDIUM_OPTIONS.find(o => o.value === m)?.label ?? m)
          .join(' or ')}`

  const days = rule.threshold_days
  return `${types} ${scope} not accessed ${mediums} for ${String(days)} day${
    days === 1 ? '' : 's'
  }`
}
