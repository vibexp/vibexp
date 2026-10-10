import {
  ArrowDown,
  ArrowUp,
  Pencil,
  Plug,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
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
import { Switch } from '@/components/ui/switch'
import {
  type AdminAuthProvider,
  type AdminAuthProviderTestResult,
  authSettingsService,
} from '@/services/authSettingsService'

import {
  availableProviderTypes,
  lockoutRiskCopy,
  PROVIDER_TYPE_LABELS,
} from './authSettingsForm'
import { ProviderDialog } from './ProviderDialog'
import type { AuthProvidersState } from './useAuthProviders'

const HEALTH_LABELS: Record<AdminAuthProvider['health']['status'], string> = {
  healthy: 'Healthy',
  unhealthy: 'Unhealthy',
  disabled: 'Disabled',
  unknown: 'Unknown',
}

const HEALTH_VARIANTS: Record<
  AdminAuthProvider['health']['status'],
  'default' | 'destructive' | 'secondary' | 'outline'
> = {
  healthy: 'default',
  unhealthy: 'destructive',
  disabled: 'secondary',
  unknown: 'outline',
}

/** A stored provider's last test, shown under its row until the next change. */
type RowTest =
  | { id: string; result: AdminAuthProviderTestResult }
  | { id: string; error: string }

function rowTestMessage(test: RowTest): { ok: boolean; text: string } {
  if ('error' in test) return { ok: false, text: test.error }
  if (test.result.is_valid) {
    return { ok: true, text: 'The test passed: the provider can be built.' }
  }
  return {
    ok: false,
    text:
      test.result.message ?? 'The test failed: the provider cannot be built.',
  }
}

function ProviderRow({
  provider,
  index,
  count,
  state,
  testing,
  test,
  onTest,
  onEdit,
  onDelete,
}: Readonly<{
  provider: AdminAuthProvider
  index: number
  count: number
  state: AuthProvidersState
  testing: boolean
  test: RowTest | undefined
  onTest: () => void
  onEdit: () => void
  onDelete: () => void
}>) {
  const disabled = state.busy || state.conflict
  const { status, last_error: lastError } = provider.health
  const tested = test ? rowTestMessage(test) : null
  return (
    <li
      className="space-y-2 py-3"
      data-testid="auth-provider-row"
      data-provider-slug={provider.slug}
    >
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex flex-col">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-6"
            aria-label={`Move ${provider.display_name} up`}
            disabled={disabled || index === 0}
            onClick={() => {
              void state.move(index, -1)
            }}
          >
            <ArrowUp className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-6"
            aria-label={`Move ${provider.display_name} down`}
            disabled={disabled || index === count - 1}
            onClick={() => {
              void state.move(index, 1)
            }}
          >
            <ArrowDown className="size-4" />
          </Button>
        </div>

        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{provider.display_name}</span>
            <Badge variant="outline">
              {PROVIDER_TYPE_LABELS[provider.type]}
            </Badge>
            <Badge
              variant={HEALTH_VARIANTS[status]}
              data-testid="auth-provider-health"
            >
              {HEALTH_LABELS[status]}
            </Badge>
          </div>
          <p className="text-muted-foreground truncate font-mono text-xs">
            {provider.slug}
            {provider.issuer_url ? ` · ${provider.issuer_url}` : ''}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Switch
            checked={provider.enabled}
            disabled={disabled}
            aria-label={`${provider.display_name} enabled`}
            onCheckedChange={enabled => {
              void state.setEnabled(provider, enabled)
            }}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled || testing}
            aria-label={`Test ${provider.display_name}`}
            onClick={onTest}
          >
            <Plug className="mr-2 size-4" />
            {testing ? 'Testing…' : 'Test'}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            disabled={disabled}
            aria-label={`Edit ${provider.display_name}`}
            onClick={onEdit}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            disabled={disabled}
            aria-label={`Delete ${provider.display_name}`}
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>

      {status === 'unhealthy' && (
        <p
          className="text-destructive text-xs"
          data-testid="auth-provider-error"
        >
          Not offered for sign-in: {lastError ?? 'it could not be built.'}
        </p>
      )}
      {tested && (
        <p
          role="status"
          className={
            tested.ok
              ? 'text-muted-foreground text-xs'
              : 'text-destructive text-xs'
          }
          data-testid="auth-provider-test"
        >
          {tested.text}
        </p>
      )}
    </li>
  )
}

/**
 * Identity providers (#1239): the stored sign-in providers with their type,
 * health and enabled switch, in sign-in page order, plus add, edit, test,
 * reorder and delete.
 *
 * Every change goes through `useAuthProviders`, which owns the shared version,
 * the reload prompt of a version conflict and the lockout-risk confirmation.
 */
export function ProvidersSection({
  state,
}: Readonly<{ state: AuthProvidersState }>) {
  const [dialog, setDialog] = useState<
    { open: false } | { open: true; stored: AdminAuthProvider | null }
  >({ open: false })
  const [deleting, setDeleting] = useState<AdminAuthProvider | null>(null)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [rowTest, setRowTest] = useState<RowTest | null>(null)

  const { providers, pendingLockout } = state

  if (state.loading && !providers) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!providers) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {state.loadError ?? 'Failed to load the sign-in providers'}
        </AlertDescription>
      </Alert>
    )
  }

  const types = availableProviderTypes(providers)
  const lockout = pendingLockout ? lockoutRiskCopy(pendingLockout.reason) : null

  const testStored = async (provider: AdminAuthProvider) => {
    try {
      setTestingId(provider.id)
      setRowTest(null)
      const result = await authSettingsService.testProvider({ id: provider.id })
      setRowTest({ id: provider.id, result })
    } catch (err) {
      setRowTest({
        id: provider.id,
        error:
          err instanceof Error ? err.message : 'Failed to test the provider',
      })
    } finally {
      setTestingId(null)
    }
  }

  return (
    <Card data-testid="auth-providers-section">
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1.5">
          <CardTitle>Identity providers</CardTitle>
          <CardDescription>
            Who people can sign in through, in the order the sign-in page lists
            them. Changes apply to the next sign-in, with no restart.
          </CardDescription>
        </div>
        <Button
          type="button"
          size="sm"
          disabled={state.busy || state.conflict}
          onClick={() => {
            setDialog({ open: true, stored: null })
          }}
        >
          <Plus className="mr-2 size-4" />
          Add provider
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {state.loadError && (
          <Alert variant="destructive" data-testid="auth-providers-load-error">
            <AlertTitle>Could not refresh the sign-in providers</AlertTitle>
            <AlertDescription>
              {state.loadError} The list below may be out of date.
            </AlertDescription>
          </Alert>
        )}

        {state.conflict && (
          <Alert variant="destructive" data-testid="auth-providers-conflict">
            <AlertTitle>
              Someone else changed the authentication settings
            </AlertTitle>
            <AlertDescription className="space-y-2">
              <p>
                Your change was not saved, so nothing was overwritten. Reload to
                see the current providers, then make the change again.
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={state.loading}
                onClick={() => {
                  void state.reload()
                }}
              >
                <RefreshCw className="mr-2 size-4" />
                Reload
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {state.actionError && (
          <Alert variant="destructive" data-testid="auth-providers-error">
            <AlertTitle>Not saved</AlertTitle>
            <AlertDescription>{state.actionError}</AlertDescription>
          </Alert>
        )}

        {providers.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="auth-providers-empty"
          >
            No sign-in provider is stored yet. Add one so people can sign in.
          </p>
        ) : (
          <ul className="divide-y">
            {providers.map((provider, index) => (
              <ProviderRow
                key={provider.id}
                provider={provider}
                index={index}
                count={providers.length}
                state={state}
                testing={testingId === provider.id}
                test={rowTest?.id === provider.id ? rowTest : undefined}
                onTest={() => {
                  void testStored(provider)
                }}
                onEdit={() => {
                  setDialog({ open: true, stored: provider })
                }}
                onDelete={() => {
                  setDeleting(provider)
                }}
              />
            ))}
          </ul>
        )}
      </CardContent>

      <ProviderDialog
        open={dialog.open}
        onOpenChange={open => {
          if (!open) setDialog({ open: false })
        }}
        stored={dialog.open ? dialog.stored : null}
        types={types}
        state={state}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={open => {
          if (!open) setDeleting(null)
        }}
        title={`Delete ${deleting?.display_name ?? 'this provider'}?`}
        description="People can no longer sign in through it, and its stored client secret is removed. This cannot be undone."
        confirmLabel="Delete provider"
        variant="destructive"
        loading={state.busy}
        onConfirm={async () => {
          const target = deleting
          setDeleting(null)
          if (target) await state.remove(target)
        }}
      />

      <ConfirmDialog
        open={lockout !== null}
        onOpenChange={open => {
          if (!open) state.cancelLockout()
        }}
        title={lockout?.title ?? ''}
        description={lockout?.description}
        confirmLabel="Apply anyway"
        variant="destructive"
        loading={state.busy}
        onConfirm={state.confirmLockout}
      />
    </Card>
  )
}
