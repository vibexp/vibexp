import { Plug } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'

import { CopyButton } from '@/components/CopyButton'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  type AdminAuthProvider,
  type AdminAuthProviderTestResult,
  type AdminAuthProviderType,
  authSettingsService,
} from '@/services/authSettingsService'

import { FieldFrame, SwitchSettingField } from '../InstanceSettingsFields'
import {
  fieldDescribedBy,
  type FieldErrors,
  serverFieldErrors,
} from '../instanceSettingsForm'
import {
  emptyProviderForm,
  issuerChanged,
  messageOf,
  nextSortOrder,
  PROVIDER_FORM_FIELDS,
  PROVIDER_TYPE_LABELS,
  type ProviderField,
  type ProviderForm,
  redirectUriFor,
  toProviderCreate,
  toProviderForm,
  toProviderTest,
  toProviderUpdate,
  validateProviderForm,
} from './authSettingsForm'
import type { AuthProvidersState } from './useAuthProviders'

function TextField({
  id,
  label,
  value,
  onChange,
  hint,
  error,
  disabled,
  type = 'text',
  autoComplete = 'off',
  placeholder,
}: Readonly<{
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  hint?: ReactNode
  error?: string
  disabled?: boolean
  type?: 'text' | 'password' | 'url'
  autoComplete?: string
  placeholder?: string
}>) {
  return (
    <FieldFrame id={id} label={label} hint={hint} error={error}>
      <Input
        id={id}
        type={type}
        value={value}
        disabled={disabled}
        autoComplete={autoComplete}
        placeholder={placeholder}
        aria-invalid={error ? true : undefined}
        aria-describedby={fieldDescribedBy(id, hint, error)}
        onChange={event => {
          onChange(event.target.value)
        }}
      />
    </FieldFrame>
  )
}

function TypePicker({
  types,
  value,
  disabled,
  onChange,
}: Readonly<{
  types: readonly AdminAuthProviderType[]
  value: AdminAuthProviderType
  disabled: boolean
  onChange: (type: AdminAuthProviderType) => void
}>) {
  return (
    <fieldset className="min-w-0 space-y-1.5">
      <legend className="mb-1.5 text-sm font-medium leading-none">
        Provider type
      </legend>
      <div className="flex flex-wrap gap-4">
        {types.map(type => (
          <label key={type} className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name="provider-type"
              value={type}
              checked={value === type}
              disabled={disabled}
              onChange={() => {
                onChange(type)
              }}
            />
            {PROVIDER_TYPE_LABELS[type]}
          </label>
        ))}
      </div>
      <p className="text-muted-foreground text-xs">
        Google and GitHub can each be added once. The type cannot be changed
        later.
      </p>
    </fieldset>
  )
}

/** What the client secret field says about leaving it blank. */
function secretHintFor(
  form: ProviderForm,
  stored: AdminAuthProvider | null
): string {
  if (stored === null) return 'Stored encrypted and never shown again.'
  if (issuerChanged(form, stored)) {
    return 'The issuer URL changed, so the stored secret is not kept: enter it again.'
  }
  return 'Leave this blank to keep it.'
}

function submitLabel(busy: boolean, editing: boolean): string {
  if (busy) return 'Saving…'
  return editing ? 'Save changes' : 'Add provider'
}

export interface ProviderEditorProps {
  /** The provider being edited, or null to add one. */
  stored: AdminAuthProvider | null
  /** The types a new provider may take (ignored on an edit). */
  types: readonly AdminAuthProviderType[]
  state: AuthProvidersState
  onDone: () => void
  onCancel: () => void
}

/**
 * The add/edit form of a sign-in provider (#1239). Kept apart from the dialog
 * that frames it so it can be rendered on its own.
 *
 * The type and slug are set once, at creation. The client secret is
 * write-only: an edit starts with it empty, and leaving it empty keeps the
 * stored one — except across an issuer URL change, where the server does not
 * keep it and the form requires it again.
 */
export function ProviderEditor({
  stored,
  types,
  state,
  onDone,
  onCancel,
}: Readonly<ProviderEditorProps>) {
  const [form, setForm] = useState<ProviderForm>(() =>
    stored ? toProviderForm(stored) : emptyProviderForm(types[0] ?? 'oidc')
  )
  const [serverErrors, setServerErrors] = useState<FieldErrors<ProviderField>>(
    {}
  )
  const [formError, setFormError] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] =
    useState<AdminAuthProviderTestResult | null>(null)
  const [testError, setTestError] = useState<string | null>(null)

  const clientErrors = useMemo(
    () => validateProviderForm(form, stored),
    [form, stored]
  )
  // Client-side errors appear once the admin tried to save or test; a server
  // error on a field stays until that field is edited.
  const errors: FieldErrors<ProviderField> = {
    ...(submitted ? clientErrors : {}),
    ...serverErrors,
  }
  const redirectUri = redirectUriFor(
    stored ? [stored] : (state.providers ?? [])
  )
  const disabled = state.busy || testing

  const setField = <K extends keyof ProviderForm>(
    field: K,
    value: ProviderForm[K]
  ) => {
    setForm(current => ({ ...current, [field]: value }))
    setTestResult(null)
    setTestError(null)
    setServerErrors(current =>
      Object.fromEntries(
        Object.entries(current).filter(([key]) => key !== field)
      )
    )
  }

  const applyServerError = (err: unknown, fallback: string) => {
    const split = serverFieldErrors(err, PROVIDER_FORM_FIELDS)
    if (split) {
      setServerErrors(split.fields)
      setFormError(split.other.length > 0 ? split.other.join(' ') : null)
      return
    }
    setFormError(messageOf(err, fallback))
  }

  const invalid = () => {
    setSubmitted(true)
    return Object.keys(clientErrors).length > 0
  }

  const save = async () => {
    if (invalid()) return
    setFormError(null)
    const result = await state.run(async (expected, confirm) => {
      if (stored) {
        await authSettingsService.updateProvider(
          stored.id,
          toProviderUpdate(stored, form, expected, confirm)
        )
        return
      }
      await authSettingsService.createProvider(
        toProviderCreate(form, nextSortOrder(state.providers ?? []), expected)
      )
    }, onDone)
    // A conflict or a lockout risk is the section's to show, behind this form.
    if (result.status === 'conflict' || result.status === 'lockout') onCancel()
    if (result.status === 'error') {
      applyServerError(result.error, 'Failed to save the provider')
    }
  }

  const test = async () => {
    if (invalid()) return
    try {
      setTesting(true)
      setTestError(null)
      setTestResult(null)
      setTestResult(
        await authSettingsService.testProvider(toProviderTest(form, stored))
      )
    } catch (err) {
      const split = serverFieldErrors(err, PROVIDER_FORM_FIELDS)
      if (split) {
        setServerErrors(split.fields)
        setTestError(split.other.length > 0 ? split.other.join(' ') : null)
        return
      }
      setTestError(messageOf(err, 'Failed to test the provider'))
    } finally {
      setTesting(false)
    }
  }

  const secretHint = secretHintFor(form, stored)

  return (
    <form
      aria-label={stored ? `Edit ${stored.display_name}` : 'Add a provider'}
      className="space-y-4"
      noValidate
      onSubmit={event => {
        event.preventDefault()
        void save()
      }}
    >
      {stored === null ? (
        <TypePicker
          types={types}
          value={form.type}
          disabled={disabled}
          onChange={type => {
            setForm(emptyProviderForm(type))
            setServerErrors({})
            setSubmitted(false)
            setTestResult(null)
            setTestError(null)
          }}
        />
      ) : (
        <p className="text-muted-foreground text-sm">
          {PROVIDER_TYPE_LABELS[stored.type]} provider{' '}
          <code className="font-mono">{stored.slug}</code>. The type and slug
          cannot be changed.
        </p>
      )}

      {stored === null && (
        <TextField
          id="slug"
          label="Slug"
          value={form.slug}
          error={errors.slug}
          disabled={disabled}
          hint="Used in the sign-in URL. Lower-case letters, digits and hyphens. It cannot be changed later."
          placeholder="okta"
          onChange={value => {
            setField('slug', value)
          }}
        />
      )}

      <TextField
        id="display_name"
        label="Display name"
        value={form.display_name}
        error={errors.display_name}
        disabled={disabled}
        hint="Shown on the sign-in button."
        onChange={value => {
          setField('display_name', value)
        }}
      />

      {form.type === 'oidc' && (
        <TextField
          id="issuer_url"
          label="Issuer URL"
          type="url"
          value={form.issuer_url}
          error={errors.issuer_url}
          disabled={disabled}
          hint="The OpenID Connect issuer, without the /.well-known path."
          placeholder="https://example.okta.com"
          onChange={value => {
            setField('issuer_url', value)
          }}
        />
      )}

      <TextField
        id="client_id"
        label="Client ID"
        value={form.client_id}
        error={errors.client_id}
        disabled={disabled}
        onChange={value => {
          setField('client_id', value)
        }}
      />

      <TextField
        id="client_secret"
        label="Client secret"
        type="password"
        autoComplete="new-password"
        value={form.client_secret}
        error={errors.client_secret}
        disabled={disabled}
        hint={secretHint}
        onChange={value => {
          setField('client_secret', value)
        }}
      />

      <div className="space-y-1">
        <label htmlFor="redirect_uri" className="text-sm font-medium">
          Redirect URI
        </label>
        <div className="flex items-center gap-2">
          <Input
            id="redirect_uri"
            readOnly
            value={redirectUri.value}
            aria-describedby="redirect_uri-hint"
            className="font-mono text-xs"
          />
          <CopyButton
            value={redirectUri.value}
            variant="outline"
            label="Copy redirect URI"
            testId="copy-redirect-uri"
          />
        </div>
        <p id="redirect_uri-hint" className="text-muted-foreground text-xs">
          Register this in the identity provider&apos;s console. It is the same
          for every provider and cannot be changed here.
          {redirectUri.derived &&
            ' It is derived from the address you are on; the saved provider shows the exact value.'}
        </p>
      </div>

      <SwitchSettingField
        id="enabled"
        label="Enabled"
        hint="Offered on the sign-in page."
        checked={form.enabled}
        disabled={disabled}
        onChange={checked => {
          setField('enabled', checked)
        }}
      />

      {testResult && (
        <Alert
          variant={testResult.is_valid ? 'default' : 'destructive'}
          data-testid="provider-test-result"
        >
          <AlertTitle>
            {testResult.is_valid ? 'The test passed' : 'The test failed'}
          </AlertTitle>
          <AlertDescription>
            {testResult.is_valid
              ? 'The server could build the provider with this configuration. Nothing was saved.'
              : (testResult.message ??
                'The server could not build the provider with this configuration.')}
          </AlertDescription>
        </Alert>
      )}
      {testError && (
        <Alert variant="destructive" data-testid="provider-test-error">
          <AlertTitle>Not tested</AlertTitle>
          <AlertDescription>{testError}</AlertDescription>
        </Alert>
      )}
      {formError && (
        <Alert variant="destructive" data-testid="provider-form-error">
          <AlertTitle>Not saved</AlertTitle>
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() => {
            void test()
          }}
        >
          <Plug className="mr-2 size-4" />
          {testing ? 'Testing…' : 'Test'}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={state.busy}
          onClick={onCancel}
        >
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={disabled || state.conflict}>
          {submitLabel(state.busy, stored !== null)}
        </Button>
      </div>
    </form>
  )
}

export interface ProviderDialogProps extends Omit<
  ProviderEditorProps,
  'onDone' | 'onCancel'
> {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** `ProviderEditor` in a dialog; the form is remounted each time it opens. */
export function ProviderDialog({
  open,
  onOpenChange,
  stored,
  types,
  state,
}: Readonly<ProviderDialogProps>) {
  const close = () => {
    onOpenChange(false)
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {stored ? 'Edit sign-in provider' : 'Add a sign-in provider'}
          </DialogTitle>
          <DialogDescription>
            {stored
              ? 'Changes apply to the next sign-in, with no restart.'
              : 'People sign in through it as soon as it is saved and enabled.'}
          </DialogDescription>
        </DialogHeader>
        {open && (
          <ProviderEditor
            stored={stored}
            types={types}
            state={state}
            onDone={close}
            onCancel={close}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
