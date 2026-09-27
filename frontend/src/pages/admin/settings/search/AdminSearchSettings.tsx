import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  type AdminInstanceSearchSettings,
  type AdminInstanceSettingsAuditParams,
  adminSettingsService,
} from '@/services/adminSettingsService'

import {
  NumberSettingField,
  SwitchSettingField,
} from '../InstanceSettingsFields'
import { InstanceSettingsLayout } from '../InstanceSettingsLayout'
import {
  type InstanceSettingsEditorOptions,
  useInstanceSettingsEditor,
} from '../useInstanceSettingsEditor'
import {
  describeSearchValues,
  SEARCH_AUDIT_FIELDS,
  SEARCH_FORM_FIELDS,
  type SearchInstanceForm,
  toSearchForm,
  toSearchUpdate,
  validateSearchForm,
  WEIGHT_FIELDS,
  WEIGHT_LABELS,
} from './searchInstanceForm'

const NOUN = 'search settings'

// Module-level so the editor's effect dependencies stay stable.
const EDITOR_OPTIONS: InstanceSettingsEditorOptions<
  AdminInstanceSearchSettings,
  SearchInstanceForm,
  ReturnType<typeof toSearchUpdate>
> = {
  get: () => adminSettingsService.getSearchSettings(),
  update: body => adminSettingsService.updateSearchSettings(body),
  reset: () => adminSettingsService.resetSearchSettings(),
  toForm: toSearchForm,
  toUpdate: toSearchUpdate,
  validate: validateSearchForm,
  fields: SEARCH_FORM_FIELDS,
  noun: NOUN,
}

const AUDIT = {
  fetchPage: (params: AdminInstanceSettingsAuditParams) =>
    adminSettingsService.listSearchSettingsAudit(params),
  fields: SEARCH_AUDIT_FIELDS,
  description: 'Every change to the instance search ranking defaults.',
  entryTestId: 'instance-search-audit-entry',
}

/**
 * Admin → Settings → Search (#1202): the instance search ranking defaults
 * every team without its own search settings ranks with, plus the
 * instance-only candidate cap.
 */
export function AdminSearchSettings() {
  const editor = useInstanceSettingsEditor(EDITOR_OPTIONS)
  const { errors, setField } = editor

  return (
    <InstanceSettingsLayout
      editor={editor}
      noun={NOUN}
      formLabel="Instance search settings"
      overrideNote="They rank with their own settings, so a change to the defaults here does not reach them — only the candidate cap does."
      describeDefaults={describeSearchValues}
      audit={AUDIT}
    >
      {(form, settings, disabled) => (
        <>
          <Card>
            <CardHeader>
              <CardTitle>Ranking defaults</CardTitle>
              <CardDescription>
                How search orders results for every team without its own search
                settings.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <SwitchSettingField
                id="recency_ranking_enabled"
                label="Recency ranking"
                hint="Re-rank results by how recently they were created and updated, not by relevance alone."
                checked={form.recency_ranking_enabled}
                error={errors.recency_ranking_enabled}
                disabled={disabled}
                onChange={checked => {
                  setField('recency_ranking_enabled', checked)
                }}
              />
              <div className="grid gap-4 sm:grid-cols-3">
                {WEIGHT_FIELDS.map(field => (
                  <NumberSettingField
                    key={field}
                    id={field}
                    label={WEIGHT_LABELS[field]}
                    value={form[field]}
                    error={errors[field]}
                    min={settings.limits.rank_weight_min}
                    step="any"
                    disabled={disabled}
                    onChange={value => {
                      setField(field, value)
                    }}
                  />
                ))}
              </div>
              <p className="text-muted-foreground text-xs">
                The weights are normalized by their sum, so only their ratio
                matters. They must not all be zero.
              </p>
              <NumberSettingField
                id="rank_half_life_days"
                label="Half-life (days)"
                hint={`How quickly recency fades: after this many days a result's recency score halves. Up to ${String(settings.limits.rank_half_life_days_max)}.`}
                value={form.rank_half_life_days}
                error={errors.rank_half_life_days}
                max={settings.limits.rank_half_life_days_max}
                step="any"
                disabled={disabled}
                onChange={value => {
                  setField('rank_half_life_days', value)
                }}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Instance-wide</CardTitle>
              <CardDescription>
                Applies to every team, including teams with their own search
                settings.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <NumberSettingField
                id="rank_candidate_cap"
                label="Candidate cap"
                hint={`How many matches are pulled and re-ranked per search (${String(settings.limits.rank_candidate_cap_min)}–${String(settings.limits.rank_candidate_cap_max)}). With recency ranking on, results beyond it are not reachable by pagination.`}
                value={form.rank_candidate_cap}
                error={errors.rank_candidate_cap}
                min={settings.limits.rank_candidate_cap_min}
                max={settings.limits.rank_candidate_cap_max}
                step={1}
                disabled={disabled}
                onChange={value => {
                  setField('rank_candidate_cap', value)
                }}
              />
            </CardContent>
          </Card>
        </>
      )}
    </InstanceSettingsLayout>
  )
}
