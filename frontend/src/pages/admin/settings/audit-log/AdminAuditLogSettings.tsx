import { AlertTriangle } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
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
  AUDIT_LOG_AUDIT_FIELDS,
  AUDIT_LOG_FORM_FIELDS,
  type AuditLogEnvelope,
  type AuditLogSettingsForm,
  describeAuditLogReset,
  retentionLoweredWarning,
  toAuditLogEnvelope,
  toAuditLogForm,
  toAuditLogUpdate,
  validateAuditLogForm,
} from './auditLogSettingsForm'

const NOUN = 'audit log settings'

// Module-level so the editor's effect dependencies stay stable.
const EDITOR_OPTIONS: InstanceSettingsEditorOptions<
  AuditLogEnvelope,
  AuditLogSettingsForm,
  ReturnType<typeof toAuditLogUpdate>
> = {
  get: async () =>
    toAuditLogEnvelope(await adminSettingsService.getAuditLogSettings()),
  update: async body =>
    toAuditLogEnvelope(await adminSettingsService.updateAuditLogSettings(body)),
  reset: () => adminSettingsService.resetAuditLogSettings(),
  toForm: toAuditLogForm,
  toUpdate: toAuditLogUpdate,
  validate: validateAuditLogForm,
  fields: AUDIT_LOG_FORM_FIELDS,
  noun: NOUN,
}

const AUDIT = {
  fetchPage: (params: AdminInstanceSettingsAuditParams) =>
    adminSettingsService.listAuditLogSettingsAudit(params),
  fields: AUDIT_LOG_AUDIT_FIELDS,
  description: 'Every change to the audit log retention and capture settings.',
  entryTestId: 'instance-audit-log-audit-entry',
}

/**
 * Admin → Settings → Audit log (#1518): how long audit events are kept and
 * whether new ones capture the client IP address and user agent. Teams cannot
 * override these, so the page has no override note.
 */
export function AdminAuditLogSettings() {
  const editor = useInstanceSettingsEditor(EDITOR_OPTIONS)
  const { errors, setField } = editor

  return (
    <InstanceSettingsLayout
      editor={editor}
      noun={NOUN}
      formLabel="Instance audit log settings"
      describeDefaults={defaults =>
        // The layout previews a reset only once the settings are loaded.
        describeAuditLogReset(defaults, editor.settings?.values ?? defaults)
      }
      audit={AUDIT}
    >
      {(form, settings, disabled) => {
        const warning = retentionLoweredWarning(form, settings.values)
        return (
          <Card>
            <CardHeader>
              <CardTitle>Retention and capture</CardTitle>
              <CardDescription>
                Applies to every audit event recorded on this instance.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <NumberSettingField
                id="retention_weeks"
                label="Retention (weeks)"
                hint={`How long an audit event is kept (${String(settings.limits.retention_weeks_min)}–${String(settings.limits.retention_weeks_max)} weeks). A daily purge deletes older events.`}
                value={form.retention_weeks}
                error={errors.retention_weeks}
                min={settings.limits.retention_weeks_min}
                max={settings.limits.retention_weeks_max}
                step={1}
                disabled={disabled}
                onChange={value => {
                  setField('retention_weeks', value)
                }}
              />
              {warning && (
                <Alert
                  variant="destructive"
                  data-testid="audit-log-retention-warning"
                >
                  <AlertTriangle className="size-4" aria-hidden />
                  <AlertTitle>Retention is being lowered</AlertTitle>
                  <AlertDescription>{warning}</AlertDescription>
                </Alert>
              )}
              <SwitchSettingField
                id="capture_ip_ua"
                label="Capture IP address and user agent"
                hint="Store the client IP address and user agent with each new audit event. Turning this off affects new events only: events already stored keep theirs until they are purged."
                checked={form.capture_ip_ua}
                error={errors.capture_ip_ua}
                disabled={disabled}
                onChange={checked => {
                  setField('capture_ip_ua', checked)
                }}
              />
            </CardContent>
          </Card>
        )
      }}
    </InstanceSettingsLayout>
  )
}
