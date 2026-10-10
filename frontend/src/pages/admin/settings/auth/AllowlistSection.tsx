import { RefreshCw, RotateCcw } from 'lucide-react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { TaxonomyInput } from '@/components/patterns/resource/form/TaxonomyInput'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { AdminAuthAllowlistImpact } from '@/services/authSettingsService'

import { FieldFrame } from '../InstanceSettingsFields'
import { fieldDescribedBy } from '../instanceSettingsForm'
import { allowlistImpactSentence } from './authSettingsForm'
import { useAllowlistEditor } from './useAllowlistEditor'

function ImpactDescription({
  impact,
}: Readonly<{ impact: AdminAuthAllowlistImpact }>) {
  return (
    <span className="block space-y-2" data-testid="allowlist-impact">
      <span className="block">
        They match neither list, so their next request ends their session and
        they cannot sign in again until the allowlist lets them. Root admins are
        never affected.
      </span>
      <span className="block font-mono text-xs">
        {impact.sample.join(', ')}
        {impact.sample_truncated && ' and more'}
      </span>
    </span>
  )
}

/**
 * Access allowlist (#1239): the email domains and addresses allowed to sign
 * in. Both lists empty means open access. The rules of a save — preview
 * first, confirm when someone would be signed out, never retry a version
 * conflict — are `useAllowlistEditor`'s.
 */
export function AllowlistSection({
  onSaved,
}: Readonly<{ onSaved?: () => void }>) {
  const editor = useAllowlistEditor(onSaved)
  const {
    stored,
    form,
    loading,
    loadError,
    busy,
    conflict,
    errors,
    hasErrors,
    dirty,
    formError,
    impact,
    confirmReset,
    setConfirmReset,
    setList,
    save,
    store,
    reset,
  } = editor
  const load = editor.reload

  if (loading && !stored) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!stored) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {loadError ?? 'Failed to load the access allowlist'}
        </AlertDescription>
      </Alert>
    )
  }

  const disabled = busy || loading

  return (
    <Card data-testid="auth-allowlist-section">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>Access allowlist</CardTitle>
          <Badge
            variant={stored.active ? 'default' : 'secondary'}
            data-testid="allowlist-status"
          >
            {stored.active ? 'Restricted' : 'Open access'}
          </Badge>
        </div>
        <CardDescription>
          Who may sign in. With both lists empty, anyone the identity providers
          accept can. Otherwise an email must match a domain or be listed
          itself. Root admins can always sign in.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          aria-label="Access allowlist"
          className="space-y-4"
          onSubmit={event => {
            event.preventDefault()
            void save()
          }}
        >
          {loadError && (
            <Alert variant="destructive" data-testid="allowlist-load-error">
              <AlertTitle>Could not refresh the access allowlist</AlertTitle>
              <AlertDescription>
                {loadError} The values below may be out of date.
              </AlertDescription>
            </Alert>
          )}

          {conflict && (
            <Alert variant="destructive" data-testid="allowlist-conflict">
              <AlertTitle>Someone else changed the access allowlist</AlertTitle>
              <AlertDescription className="space-y-2">
                <p>
                  Your changes were not saved, so nothing was overwritten.
                  Reload to see the current lists; your unsaved edits will be
                  discarded.
                </p>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={loading}
                  onClick={() => {
                    void load()
                  }}
                >
                  <RefreshCw className="mr-2 size-4" />
                  Reload
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <FieldFrame
            id="allowlist-domains"
            label="Allowed domains"
            hint="Every address at the domain may sign in. Press Enter or comma to add one."
            error={errors.domains}
          >
            <TaxonomyInput
              id="allowlist-domains"
              value={form.domains}
              disabled={disabled}
              placeholder="example.com"
              aria-invalid={errors.domains ? true : undefined}
              aria-describedby={fieldDescribedBy(
                'allowlist-domains',
                true,
                errors.domains
              )}
              onChange={next => {
                setList('domains', next)
              }}
            />
          </FieldFrame>

          <FieldFrame
            id="allowlist-emails"
            label="Allowed email addresses"
            hint="Individual addresses outside the allowed domains."
            error={errors.emails}
          >
            <TaxonomyInput
              id="allowlist-emails"
              value={form.emails}
              disabled={disabled}
              placeholder="contractor@partner.example"
              aria-invalid={errors.emails ? true : undefined}
              aria-describedby={fieldDescribedBy(
                'allowlist-emails',
                true,
                errors.emails
              )}
              onChange={next => {
                setList('emails', next)
              }}
            />
          </FieldFrame>

          {formError && (
            <Alert variant="destructive" data-testid="allowlist-error">
              <AlertTitle>Not saved</AlertTitle>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-muted-foreground text-sm">
              {dirty ? 'You have unsaved changes.' : ' '}
            </p>
            <div className="flex flex-wrap gap-2">
              {stored.version !== null && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={disabled}
                  onClick={() => {
                    setConfirmReset(true)
                  }}
                >
                  <RotateCcw className="mr-2 size-4" />
                  Reset to open access
                </Button>
              )}
              <Button
                type="submit"
                size="sm"
                disabled={disabled || !dirty || hasErrors || conflict}
              >
                {busy ? 'Saving…' : 'Save allowlist'}
              </Button>
            </div>
          </div>
        </form>
      </CardContent>

      <ConfirmDialog
        open={impact !== null}
        onOpenChange={open => {
          if (!open) editor.cancelImpact()
        }}
        title={impact ? allowlistImpactSentence(impact.count) : ''}
        description={impact ? <ImpactDescription impact={impact} /> : undefined}
        confirmLabel="Save and sign them out"
        variant="destructive"
        loading={busy}
        onConfirm={store}
      />

      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title="Reset to open access?"
        description="The stored allowlist is removed, so anyone the identity providers accept can sign in again."
        confirmLabel="Reset to open access"
        variant="destructive"
        loading={busy}
        onConfirm={reset}
      />
    </Card>
  )
}
