import { zodResolver } from '@hookform/resolvers/zod'
import {
  AlertCircle,
  CheckCircle2,
  Info,
  Loader2,
  Send,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm, type UseFormReturn } from 'react-hook-form'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  ProviderCard,
  SenderIdentityCard,
} from '@/features/email-provider/EmailProviderFields'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import {
  type AdminInstanceEmailSettings,
  type AdminInstanceEmailTestResponse,
  adminService,
} from '@/services/adminService'
import { ApiError } from '@/types/errors'

import { InstanceEmailAuditList } from './InstanceEmailAuditList'
import {
  formFieldForServerField,
  formMatchesStored,
  type InstanceEmailFormValues,
  instanceEmailSchema,
  instanceSecretError,
  toInstanceFormValues,
  toInstanceRequest,
} from './instanceEmailForm'
import { InstanceEmailStatusCard } from './InstanceEmailStatusCard'

const INSTANCE_PROVIDER_DESCRIPTION =
  'Choose where the instance sends its mail from — invitations, notifications, digests and the contact form. Teams with their own provider are unaffected.'

const INSTANCE_STORED_CREDENTIAL_HINT =
  'A credential is stored. Leave this blank to keep it — for saving, and for a test send to the same destination.'

/**
 * Admin → Settings → Email (#1191): view, configure, test, remove and audit
 * the instance email provider (#1189's endpoints). The section heading comes
 * from the admin shell (`admin-nav.ts`), so the page renders none.
 */
export function AdminEmailSettings() {
  const { handleError } = useErrorHandler()
  const [settings, setSettings] = useState<AdminInstanceEmailSettings | null>(
    null
  )
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [testResult, setTestResult] =
    useState<AdminInstanceEmailTestResponse | null>(null)
  const [auditKey, setAuditKey] = useState(0)

  const form = useForm<InstanceEmailFormValues>({
    resolver: zodResolver(instanceEmailSchema),
    defaultValues: toInstanceFormValues({
      configured: false,
      provider_type: null,
      has_credential: false,
      is_healthy: null,
    }),
  })

  /** Applies a fresh server state: the form is reset, so the secret is blank. */
  const apply = useCallback(
    (next: AdminInstanceEmailSettings) => {
      setSettings(next)
      form.reset(toInstanceFormValues(next))
    },
    [form]
  )

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      apply(await adminService.getInstanceEmailSettings())
    } catch (err) {
      setLoadError(
        err instanceof Error ? err.message : 'Failed to load the email settings'
      )
    } finally {
      setLoading(false)
    }
  }, [apply])

  useEffect(() => {
    void load()
  }, [load])

  /** A 400's field errors land on their inputs; anything else is a toast. */
  const reportError = (err: unknown, fallback: string) => {
    if (err instanceof ApiError && err.status === 400) {
      let placed = false
      for (const fieldError of err.validationErrors ?? []) {
        const name = formFieldForServerField(fieldError.field)
        if (name) {
          form.setError(name, { type: 'server', message: fieldError.message })
          placed = true
        }
      }
      if (placed) return
    }
    handleError(err, fallback)
  }

  const validateFor = async (
    action: 'save' | 'test',
    stored: AdminInstanceEmailSettings
  ): Promise<InstanceEmailFormValues | null> => {
    if (!(await form.trigger())) return null
    const values = form.getValues()
    const message = instanceSecretError(action, stored, values)
    if (message) {
      form.setError('secret', { type: 'manual', message })
      return null
    }
    return values
  }

  const handleSave = async (stored: AdminInstanceEmailSettings) => {
    const values = await validateFor('save', stored)
    if (!values) return
    try {
      setSaving(true)
      setTestResult(null)
      apply(
        await adminService.upsertInstanceEmailSettings(
          toInstanceRequest(values)
        )
      )
      setAuditKey(key => key + 1)
      toast.success('Instance email settings saved')
    } catch (err) {
      reportError(err, 'Failed to save the email settings')
    } finally {
      setSaving(false)
    }
  }

  /**
   * An untouched form of a configured instance tests the STORED configuration
   * (empty body); anything else tests the form's values as a candidate, so a
   * change can be verified before it is saved. Never wired into save: saving
   * must not send mail as a side effect.
   */
  const handleTest = async (stored: AdminInstanceEmailSettings) => {
    let body: ReturnType<typeof toInstanceRequest> | undefined
    if (!stored.configured || !formMatchesStored(stored, form.getValues())) {
      const values = await validateFor('test', stored)
      if (!values) return
      body = toInstanceRequest(values)
    }
    try {
      setTesting(true)
      setTestResult(null)
      setTestResult(await adminService.testInstanceEmailSettings(body))
    } catch (err) {
      reportError(err, 'Failed to send the test email')
    } finally {
      setTesting(false)
    }
  }

  const handleRemove = async () => {
    try {
      setRemoving(true)
      await adminService.deleteInstanceEmailSettings()
      toast.success('Instance email configuration removed')
    } catch (err) {
      // Someone else removed it first: the refetch below shows the truth.
      if (!(err instanceof ApiError && err.status === 409)) {
        handleError(err, 'Failed to remove the email configuration')
        setRemoving(false)
        return
      }
    }
    setConfirmRemove(false)
    setTestResult(null)
    setRemoving(false)
    setAuditKey(key => key + 1)
    await load()
  }

  if (loading && !settings) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!settings) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {loadError ?? 'Failed to load the email settings'}
        </AlertDescription>
      </Alert>
    )
  }

  const busy = saving || testing || removing

  return (
    <div className="space-y-6">
      <InstanceEmailStatusCard settings={settings} />

      <Form {...form}>
        <form
          onSubmit={event => {
            event.preventDefault()
            void handleSave(settings)
          }}
          className="space-y-6"
          aria-label="Instance email settings"
        >
          <ProviderCard
            form={form}
            busy={busy}
            hasCredential={settings.configured && settings.has_credential}
            description={INSTANCE_PROVIDER_DESCRIPTION}
            storedCredentialHint={INSTANCE_STORED_CREDENTIAL_HINT}
          />
          <SenderIdentityCard form={form} busy={busy}>
            <InstanceOnlyFields form={form} />
          </SenderIdentityCard>

          {testResult && <TestResultAlert result={testResult} />}

          <Card>
            <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-4">
              <p className="text-muted-foreground text-sm">
                <Info className="mr-1 inline size-4 align-text-bottom" />A test
                message always goes to your own account address.
              </p>
              <div className="flex flex-wrap gap-2">
                {settings.configured && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      setConfirmRemove(true)
                    }}
                  >
                    <Trash2 className="mr-2 size-4" />
                    Remove configuration
                  </Button>
                )}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    void handleTest(settings)
                  }}
                >
                  {testing ? (
                    <Loader2 className="mr-2 size-4 animate-spin" />
                  ) : (
                    <Send className="mr-2 size-4" />
                  )}
                  {testing ? 'Sending…' : 'Send test email'}
                </Button>
                <Button type="submit" size="sm" disabled={busy}>
                  {saving ? 'Saving…' : 'Save changes'}
                </Button>
              </div>
            </CardContent>
          </Card>
        </form>
      </Form>

      <InstanceEmailAuditList refreshKey={auditKey} />

      <ConfirmDialog
        open={confirmRemove}
        onOpenChange={setConfirmRemove}
        title="Remove the instance email configuration?"
        description="The stored provider and its credential are deleted. Mail sent by the instance — invitations, notifications, digests — is discarded until a provider is configured again. Teams with their own email provider are unaffected."
        confirmLabel="Remove"
        variant="destructive"
        loading={removing}
        onConfirm={handleRemove}
      />
    </div>
  )
}

/** The two fields only the instance has, rendered inside the sender card. */
function InstanceOnlyFields({
  form,
}: Readonly<{ form: UseFormReturn<InstanceEmailFormValues> }>) {
  return (
    <>
      <FormField
        control={form.control}
        name="contact_recipient_address"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Contact form recipient</FormLabel>
            <FormControl>
              <Input {...field} placeholder="hello@acme.test" />
            </FormControl>
            <FormDescription>
              Optional — where contact-form messages go. Blank uses the from
              address.
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="privacy_policy_url"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Privacy policy URL</FormLabel>
            <FormControl>
              <Input {...field} placeholder="https://acme.test/privacy" />
            </FormControl>
            <FormDescription>
              Optional — linked from outbound mail. Blank links none.
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

/** Outcome of a test send — a failed send is a result, not a thrown error. */
function TestResultAlert({
  result,
}: Readonly<{ result: AdminInstanceEmailTestResponse }>) {
  return (
    <Alert
      variant={result.is_valid ? 'default' : 'destructive'}
      data-testid="instance-email-test-result"
    >
      {result.is_valid ? (
        <CheckCircle2 className="size-4" />
      ) : (
        <AlertCircle className="size-4" />
      )}
      <AlertTitle>
        {result.is_valid ? 'Test email sent' : 'Test email failed'}
      </AlertTitle>
      <AlertDescription>
        <p>{result.message}</p>
        <p className="mt-1">Sent to {result.recipient}.</p>
        {result.details.error_details && (
          <p className="mt-1">Reason: {result.details.error_details}</p>
        )}
      </AlertDescription>
    </Alert>
  )
}
