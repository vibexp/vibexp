import { zodResolver } from '@hookform/resolvers/zod'
import {
  AlertCircle,
  CheckCircle2,
  Info,
  Loader2,
  RotateCcw,
  Send,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import {
  emailProviderService,
  type TeamEmailProviderResponse,
  type TeamEmailProviderTestResponse,
} from '@/services/emailProviderService'
import type { Team } from '@/services/teamService'

import { ProviderCard, SenderIdentityCard } from './EmailProviderFields'
import {
  type EmailProviderFormValues,
  emailProviderSchema,
  EMPTY_FORM,
  secretError,
  toFormValues,
  toRequest,
} from './emailProviderForm'
import { ReadOnlyCard, StatusCard } from './EmailProviderStatus'

/**
 * Team email-provider settings (#506, epic #499).
 *
 * `team` is the team `TeamScopeLayout` resolved from the URL (#540/#584). Both
 * the permission gating and every read/write MUST key on it rather than on the
 * ambient `currentTeam`: React fires child effects before parent effects, so on
 * a cold deep-link this page's load effect runs BEFORE the layout's
 * `setCurrentTeam` sync and the ambient value is still the previously persisted
 * team. `usePermissions` fails closed on `null`, so a missing team permits
 * nothing.
 *
 * The GET always returns 200 — a team with no provider is inheriting the
 * instance one — so the page renders three states off one response and never
 * handles a 404.
 */
export function EmailProvider({ team }: Readonly<{ team: Team }>) {
  const { can } = usePermissions(team)
  const canEdit = can('team.update')
  const { handleError } = useErrorHandler()

  const [provider, setProvider] = useState<TeamEmailProviderResponse | null>(
    null
  )
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [reverting, setReverting] = useState(false)
  const [confirmRevert, setConfirmRevert] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [testResult, setTestResult] =
    useState<TeamEmailProviderTestResponse | null>(null)

  const teamId = team.id

  const form = useForm<EmailProviderFormValues>({
    resolver: zodResolver(emailProviderSchema),
    defaultValues: EMPTY_FORM,
  })

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      const response = await emailProviderService.getEmailProvider(teamId)
      setProvider(response)
      // Reset rather than merge: this also blanks `secret` on every load, so an
      // existing provider always starts from "leave blank to keep current".
      form.reset(toFormValues(response))
    } catch (err) {
      setLoadError(
        err instanceof Error ? err.message : 'Failed to load the email provider'
      )
    } finally {
      setLoading(false)
    }
  }, [teamId, form])

  useEffect(() => {
    void load()
  }, [load])

  /** Both actions need a valid form; each has its own rule about the secret. */
  const validateFor = async (
    action: 'save' | 'test'
  ): Promise<EmailProviderFormValues | null> => {
    if (!(await form.trigger())) return null
    const values = form.getValues()
    const message = secretError(
      action,
      {
        configured: provider?.configured ?? false,
        providerTypeChanged:
          provider?.configured === true &&
          provider.provider_type !== values.provider_type,
      },
      values.secret
    )
    if (message) {
      form.setError('secret', { type: 'manual', message })
      return null
    }
    return values
  }

  const handleSave = async () => {
    const values = await validateFor('save')
    if (!values) return
    try {
      setSaving(true)
      setTestResult(null)
      const updated = await emailProviderService.upsertEmailProvider(
        teamId,
        toRequest(values)
      )
      setProvider(updated)
      form.reset(toFormValues(updated))
      toast.success('Email provider saved')
    } catch (err) {
      handleError(err, 'Failed to save the email provider')
    } finally {
      setSaving(false)
    }
  }

  /**
   * Testing is an explicit action, deliberately NOT wired into save (unlike
   * `ModelProviderDialog`'s validate-on-save probe): saving a mail provider
   * must never send mail as a side effect.
   */
  const handleTest = async () => {
    const values = await validateFor('test')
    if (!values) return
    try {
      setTesting(true)
      setTestResult(null)
      const result = await emailProviderService.testEmailProvider(
        teamId,
        toRequest(values)
      )
      // A rejected send comes back 200 with is_valid:false — it answers the
      // question that was asked, so it is reported inline, not as an error.
      setTestResult(result)
    } catch (err) {
      handleError(err, 'Failed to send the test email')
    } finally {
      setTesting(false)
    }
  }

  const handleRevert = async () => {
    try {
      setReverting(true)
      await emailProviderService.deleteEmailProvider(teamId)
      setConfirmRevert(false)
      setTestResult(null)
      toast.success('Reverted to the instance default')
      await load()
    } catch (err) {
      handleError(err, 'Failed to revert to the instance default')
    } finally {
      setReverting(false)
    }
  }

  const header = (
    <PageHeader
      title="Email Provider"
      description="Send this team's invitations, notifications and digests through your own email provider."
    />
  )

  if (loading) {
    return (
      <div className="space-y-6">
        {header}
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (!provider) {
    return (
      <div className="space-y-6">
        {header}
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            {loadError ?? 'Failed to load the email provider'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  if (!canEdit) {
    return (
      <div className="space-y-6">
        {header}
        <StatusCard provider={provider} />
        <ReadOnlyCard provider={provider} />
      </div>
    )
  }

  const busy = saving || testing || reverting

  return (
    <div className="space-y-6">
      {header}
      <StatusCard provider={provider} />

      <Form {...form}>
        <form
          onSubmit={event => {
            event.preventDefault()
            void handleSave()
          }}
          className="space-y-6"
        >
          <ProviderCard
            form={form}
            busy={busy}
            hasCredential={provider.has_credential}
          />
          <SenderIdentityCard form={form} busy={busy} />

          {testResult && <TestResultAlert result={testResult} />}

          <Card>
            <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-4">
              <p className="text-muted-foreground text-sm">
                <Info className="mr-1 inline size-4 align-text-bottom" />A test
                message goes to your own account address.
              </p>
              <div className="flex flex-wrap gap-2">
                {provider.configured && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      setConfirmRevert(true)
                    }}
                  >
                    <RotateCcw className="mr-2 size-4" />
                    Revert to instance default
                  </Button>
                )}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    void handleTest()
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

      <ConfirmDialog
        open={confirmRevert}
        onOpenChange={setConfirmRevert}
        title="Revert to the instance default?"
        description="This team's stored provider and its credential are deleted. Mail will be sent by the instance provider instead."
        confirmLabel="Revert"
        variant="destructive"
        loading={reverting}
        onConfirm={handleRevert}
      />
    </div>
  )
}

/** Outcome of a test send — reported inline, never as a thrown error. */
function TestResultAlert({
  result,
}: Readonly<{ result: TeamEmailProviderTestResponse }>) {
  return (
    <Alert variant={result.is_valid ? 'default' : 'destructive'}>
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
