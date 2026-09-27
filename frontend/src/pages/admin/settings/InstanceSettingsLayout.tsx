import { RefreshCw, RotateCcw, Users } from 'lucide-react'
import { type ReactNode, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatDateTime } from '@/lib/time'
import type {
  AdminInstanceSettingsAuditEntry,
  AdminInstanceSettingsAuditPage,
} from '@/services/adminService'
import type { AdminInstanceSettingsAuditParams } from '@/services/adminSettingsService'

import {
  type AuditField,
  diffAuditFields,
  resettableAuditActionLabel,
} from './instanceSettingsAudit'
import { InstanceSettingsAuditList } from './InstanceSettingsAuditList'
import type {
  InstanceSettingsEditor,
  InstanceSettingsEnvelope,
} from './useInstanceSettingsEditor'

type AnyEnvelope = InstanceSettingsEnvelope<unknown, unknown>

/** "Using built-in defaults" / "Customized", the last change, the overrides. */
export function InstanceSettingsStatus({
  settings,
  overrideNote,
}: Readonly<{
  settings: AnyEnvelope
  /** What a team override means for this section, after the count. */
  overrideNote: string
}>) {
  const customized = settings.source === 'instance'
  const overrides = settings.teams_with_override
  return (
    <Card data-testid="instance-settings-status">
      <CardContent className="space-y-2 pt-4 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={customized ? 'default' : 'secondary'}
            data-testid="instance-settings-source"
          >
            {customized ? 'Customized' : 'Using built-in defaults'}
          </Badge>
          {settings.updated_at && (
            <span className="text-muted-foreground">
              Last changed
              {settings.updated_by_name
                ? ` by ${settings.updated_by_name}`
                : ''}{' '}
              at {formatDateTime(settings.updated_at)}
            </span>
          )}
        </div>
        <p
          className="text-muted-foreground flex items-start gap-1"
          data-testid="instance-settings-overrides"
        >
          <Users className="mt-0.5 size-4 shrink-0" />
          {overrides === 0
            ? 'No team overrides these settings, so a change here reaches every team.'
            : `${String(overrides)} ${overrides === 1 ? 'team overrides' : 'teams override'} these settings. ${overrideNote}`}
        </p>
      </CardContent>
    </Card>
  )
}

export interface InstanceSettingsLayoutProps<
  S extends InstanceSettingsEnvelope<S['values'], S['limits']>,
  F extends object,
  B,
> {
  editor: InstanceSettingsEditor<S, F, B>
  /** e.g. "search settings", for messages. */
  noun: string
  /** Accessible name of the form. */
  formLabel: string
  overrideNote: string
  /** A one-line summary of the built-in defaults, previewed before a reset. */
  describeDefaults: (values: S['values']) => string
  /** The form's cards, rendered from the loaded state. */
  children: (form: F, settings: S, disabled: boolean) => ReactNode
  audit: {
    fetchPage: (
      params: AdminInstanceSettingsAuditParams
    ) => Promise<AdminInstanceSettingsAuditPage>
    fields: readonly AuditField[]
    description: string
    entryTestId: string
  }
}

/**
 * The page frame shared by Admin → Settings → Search and AI Summary (#1202):
 * loading and load-error states, the status card, the version-conflict
 * prompt, the form's action bar, the reset confirmation (with the built-in
 * defaults previewed) and the change history. The section heading comes from
 * the admin shell (`admin-nav.ts`), so the page renders none.
 */
export function InstanceSettingsLayout<
  S extends InstanceSettingsEnvelope<S['values'], S['limits']>,
  F extends object,
  B,
>({
  editor,
  noun,
  formLabel,
  overrideNote,
  describeDefaults,
  children,
  audit,
}: Readonly<InstanceSettingsLayoutProps<S, F, B>>) {
  const [confirmReset, setConfirmReset] = useState(false)
  const { settings, form } = editor

  if (editor.loading && !settings) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!settings || !form) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {editor.loadError ?? `Failed to load the ${noun}`}
        </AlertDescription>
      </Alert>
    )
  }

  const busy = editor.saving || editor.resetting || editor.loading

  return (
    <div className="space-y-6">
      <InstanceSettingsStatus settings={settings} overrideNote={overrideNote} />

      {editor.loadError && (
        <Alert variant="destructive" data-testid="instance-settings-load-error">
          <AlertTitle>Could not refresh the {noun}</AlertTitle>
          <AlertDescription>
            {editor.loadError} The values below may be out of date.
          </AlertDescription>
        </Alert>
      )}

      {editor.conflict && (
        <Alert variant="destructive" data-testid="instance-settings-conflict">
          <AlertTitle>Someone else changed these settings</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              Your changes were not saved, so nothing was overwritten. Reload to
              see the current values; your unsaved edits will be discarded.
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={editor.loading}
              onClick={() => {
                void editor.reload()
              }}
            >
              <RefreshCw className="mr-2 size-4" />
              Reload
            </Button>
          </AlertDescription>
        </Alert>
      )}

      <form
        aria-label={formLabel}
        className="space-y-6"
        onSubmit={event => {
          event.preventDefault()
          void editor.save()
        }}
      >
        {children(form, settings, busy)}

        {editor.formError && (
          <Alert variant="destructive" data-testid="instance-settings-error">
            <AlertTitle>Not saved</AlertTitle>
            <AlertDescription>{editor.formError}</AlertDescription>
          </Alert>
        )}

        <Card>
          <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-4">
            <p className="text-muted-foreground text-sm">
              {editor.dirty ? 'You have unsaved changes.' : ' '}
            </p>
            <div className="flex flex-wrap gap-2">
              {settings.source === 'instance' && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    setConfirmReset(true)
                  }}
                >
                  <RotateCcw className="mr-2 size-4" />
                  Reset to defaults
                </Button>
              )}
              <Button
                type="submit"
                size="sm"
                disabled={
                  busy || !editor.dirty || editor.hasErrors || editor.conflict
                }
              >
                {editor.saving ? 'Saving…' : 'Save changes'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>

      <InstanceSettingsAuditList
        fetchPage={audit.fetchPage}
        describeChanges={(entry: AdminInstanceSettingsAuditEntry) =>
          diffAuditFields(entry, audit.fields)
        }
        actionLabel={resettableAuditActionLabel}
        description={audit.description}
        entryTestId={audit.entryTestId}
        refreshKey={editor.auditKey}
      />

      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={`Reset the ${noun} to the built-in defaults?`}
        description={`The stored values are removed and the built-in defaults apply again: ${describeDefaults(settings.built_in_defaults)}.`}
        confirmLabel="Reset to defaults"
        variant="destructive"
        loading={editor.resetting}
        onConfirm={async () => {
          // A failure is shown as the form-level error, so close either way.
          await editor.resetToDefaults()
          setConfirmReset(false)
        }}
      />
    </div>
  )
}
