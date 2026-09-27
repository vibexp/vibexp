import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { AI_SUMMARY_STYLES } from '@/pages/teams/settings/model-providers/aiSummaryForm'
import {
  type AdminInstanceAISummarySettings,
  type AdminInstanceSettingsAuditParams,
  adminSettingsService,
} from '@/services/adminSettingsService'

import {
  FieldFrame,
  NumberSettingField,
  SwitchSettingField,
} from '../InstanceSettingsFields'
import { fieldDescribedBy } from '../instanceSettingsForm'
import { InstanceSettingsLayout } from '../InstanceSettingsLayout'
import {
  type InstanceSettingsEditorOptions,
  useInstanceSettingsEditor,
} from '../useInstanceSettingsEditor'
import {
  AI_SUMMARY_AUDIT_FIELDS,
  AI_SUMMARY_FORM_FIELDS,
  type AISummaryInstanceForm,
  type AISummaryStyle,
  describeAISummaryValues,
  toAISummaryForm,
  toAISummaryUpdate,
  validateAISummaryForm,
} from './aiSummaryInstanceForm'

const NOUN = 'AI summary settings'

// Native <select> rather than the Radix one, as on the team AI summary card:
// it needs no jsdom layout shims.
const selectClass =
  'border-input bg-background focus-visible:ring-ring h-9 w-full rounded-md border px-3 py-1 text-sm focus-visible:ring-1 focus-visible:outline-none'

// Module-level so the editor's effect dependencies stay stable.
const EDITOR_OPTIONS: InstanceSettingsEditorOptions<
  AdminInstanceAISummarySettings,
  AISummaryInstanceForm,
  ReturnType<typeof toAISummaryUpdate>
> = {
  get: () => adminSettingsService.getAISummarySettings(),
  update: body => adminSettingsService.updateAISummarySettings(body),
  reset: () => adminSettingsService.resetAISummarySettings(),
  toForm: toAISummaryForm,
  toUpdate: toAISummaryUpdate,
  validate: validateAISummaryForm,
  fields: AI_SUMMARY_FORM_FIELDS,
  noun: NOUN,
}

const AUDIT = {
  fetchPage: (params: AdminInstanceSettingsAuditParams) =>
    adminSettingsService.listAISummarySettingsAudit(params),
  fields: AI_SUMMARY_AUDIT_FIELDS,
  description:
    'Every change to the instance AI summary defaults and server budgets.',
  entryTestId: 'instance-ai-summary-audit-entry',
}

const STYLE_HINT = 'How detailed a summary is.'

/**
 * Admin → Settings → AI Summary (#1202): the AI summary defaults every team
 * without its own AI summary settings uses, plus the server budgets that
 * apply to every team.
 */
export function AdminAISummarySettings() {
  const editor = useInstanceSettingsEditor(EDITOR_OPTIONS)
  const { errors, setField } = editor

  return (
    <InstanceSettingsLayout
      editor={editor}
      noun={NOUN}
      formLabel="Instance AI summary settings"
      overrideNote="They use their own settings, so a change to the team defaults here does not reach them — the server budgets still do."
      describeDefaults={describeAISummaryValues}
      audit={AUDIT}
    >
      {(form, settings, disabled) => {
        const { limits } = settings
        return (
          <>
            <Card>
              <CardHeader>
                <CardTitle>Team defaults</CardTitle>
                <CardDescription>
                  What every team without its own AI summary settings uses.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <SwitchSettingField
                  id="enabled"
                  label="Enable AI Summary"
                  hint="Summarize the top search results with the team's model provider."
                  checked={form.enabled}
                  error={errors.enabled}
                  disabled={disabled}
                  onChange={checked => {
                    setField('enabled', checked)
                  }}
                />
                <div className="grid gap-4 sm:grid-cols-3">
                  <NumberSettingField
                    id="top_n"
                    label="Results to read"
                    hint={`How many top-ranked results a summary reads (${String(limits.top_n_min)}–${String(limits.top_n_max)}).`}
                    value={form.top_n}
                    error={errors.top_n}
                    min={limits.top_n_min}
                    max={limits.top_n_max}
                    step={1}
                    disabled={disabled}
                    onChange={value => {
                      setField('top_n', value)
                    }}
                  />
                  <FieldFrame
                    id="style"
                    label="Style"
                    hint={STYLE_HINT}
                    error={errors.style}
                  >
                    <select
                      id="style"
                      className={selectClass}
                      value={form.style}
                      disabled={disabled}
                      aria-invalid={errors.style ? true : undefined}
                      aria-describedby={fieldDescribedBy(
                        'style',
                        STYLE_HINT,
                        errors.style
                      )}
                      onChange={e => {
                        setField('style', e.target.value as AISummaryStyle)
                      }}
                    >
                      {AI_SUMMARY_STYLES.map(style => (
                        <option key={style.id} value={style.id}>
                          {style.label}
                        </option>
                      ))}
                    </select>
                  </FieldFrame>
                  <NumberSettingField
                    id="max_output_tokens"
                    label="Response length (tokens)"
                    hint={`Upper bound on how much a summary may write (${String(limits.max_output_tokens_min)}–${String(limits.max_output_tokens_max)}).`}
                    value={form.max_output_tokens}
                    error={errors.max_output_tokens}
                    min={limits.max_output_tokens_min}
                    max={limits.max_output_tokens_max}
                    step={1}
                    disabled={disabled}
                    onChange={value => {
                      setField('max_output_tokens', value)
                    }}
                  />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Advanced / server budgets</CardTitle>
                <CardDescription>
                  These apply to every team, including teams with their own AI
                  summary settings: they bound how much text one summary reads
                  and how long the server waits for the model.
                </CardDescription>
              </CardHeader>
              <CardContent className="grid gap-4 sm:grid-cols-3">
                <NumberSettingField
                  id="per_document_chars"
                  label="Per-document budget (chars)"
                  hint="How many characters of each result a summary reads."
                  value={form.per_document_chars}
                  error={errors.per_document_chars}
                  min={limits.chars_min}
                  max={limits.chars_max}
                  step={1}
                  disabled={disabled}
                  onChange={value => {
                    setField('per_document_chars', value)
                  }}
                />
                <NumberSettingField
                  id="total_context_chars"
                  label="Total context budget (chars)"
                  hint="The character budget of one summary's whole context; at least the per-document budget."
                  value={form.total_context_chars}
                  error={errors.total_context_chars}
                  min={limits.chars_min}
                  max={limits.chars_max}
                  step={1}
                  disabled={disabled}
                  onChange={value => {
                    setField('total_context_chars', value)
                  }}
                />
                <NumberSettingField
                  id="request_timeout_ms"
                  label="Request timeout (ms)"
                  hint="How long the server waits for the model, in milliseconds."
                  value={form.request_timeout_ms}
                  error={errors.request_timeout_ms}
                  min={limits.request_timeout_ms_min}
                  max={limits.request_timeout_ms_max}
                  step={1}
                  disabled={disabled}
                  onChange={value => {
                    setField('request_timeout_ms', value)
                  }}
                />
              </CardContent>
            </Card>
          </>
        )
      }}
    </InstanceSettingsLayout>
  )
}
