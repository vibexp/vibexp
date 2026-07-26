import { AlertTriangle, Loader2 } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
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
import { toast } from '@/lib/toast'
import type { GitHubAppConfigResponse } from '@/services/githubAppConfigService'
import { githubAppConfigService } from '@/services/githubAppConfigService'

import { CopyableValue } from './CopyableValue'
import { GitHubAppConfigDialog } from './GitHubAppConfigDialog'
import { GitHubAppPostSaveDialog } from './GitHubAppPostSaveDialog'
import { GitHubAppSetupGuide } from './GitHubAppSetupGuide'
import { describeValidationFailure } from './validationMessages'

export interface GitHubAppConfigCardProps {
  teamId: string
  /** The team's registration, or null when it has none yet. */
  config: GitHubAppConfigResponse | null
  /** Whether the viewer may mutate the registration (owner/admin). */
  canManage: boolean
  /** Re-fetch after any mutation. */
  onChanged: () => void | Promise<void>
}

/**
 * The team's GitHub App registration: the setup guide when there is none, the
 * current state and its controls when there is.
 *
 * Every mutating affordance is gated on `canManage`. That gating is convenience
 * — the server authorizes every write regardless — but offering a member a
 * button that is guaranteed to 403 is its own kind of bug, and the existing
 * provider settings pages do exactly that. This one does not copy them.
 */
export function GitHubAppConfigCard({
  teamId,
  config,
  canManage,
  onChanged,
}: GitHubAppConfigCardProps) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const [postSave, setPostSave] = useState<{
    webhookUrl: string
    webhookSecret?: string
  } | null>(null)
  const [confirmRotate, setConfirmRotate] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [busy, setBusy] = useState(false)
  const [verifying, setVerifying] = useState(false)

  const handleRotate = async () => {
    setBusy(true)
    try {
      const rotated = await githubAppConfigService.rotateWebhookToken(teamId)
      setConfirmRotate(false)
      await onChanged()
      setPostSave({ webhookUrl: rotated.webhook_url })
      toast.success('Webhook URL rotated', {
        description: 'Update it on GitHub now, or deliveries will stop.',
      })
    } catch {
      toast.error('Could not rotate the webhook URL')
    } finally {
      setBusy(false)
    }
  }

  const handleDelete = async () => {
    setBusy(true)
    try {
      await githubAppConfigService.deleteAppConfig(teamId)
      setConfirmDelete(false)
      await onChanged()
      toast.success('GitHub App removed')
    } catch {
      toast.error('Could not remove the GitHub App')
    } finally {
      setBusy(false)
    }
  }

  const handleVerify = async () => {
    setVerifying(true)
    try {
      const result = await githubAppConfigService.validateAppConfig(teamId)
      if (result.is_valid) {
        toast.success('GitHub App verified')
        return
      }
      const failure = describeValidationFailure(
        result.details?.error_details,
        result.message
      )
      toast.error(failure.title, { description: failure.description })
    } catch {
      toast.error('Could not verify the GitHub App')
    } finally {
      setVerifying(false)
    }
  }

  if (!config) {
    return (
      <>
        <Card>
          <CardHeader>
            <CardTitle>Connect a GitHub App</CardTitle>
            <CardDescription>
              This team has no GitHub App yet. Each team brings its own App, so
              its repositories stay under its own control.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <GitHubAppSetupGuide teamId={teamId} />
            {canManage ? (
              <Button
                onClick={() => {
                  setDialogOpen(true)
                }}
              >
                Register GitHub App
              </Button>
            ) : (
              <Alert>
                <AlertTitle>Ask an owner or admin</AlertTitle>
                <AlertDescription>
                  Registering a GitHub App needs the permission to manage this
                  team.
                </AlertDescription>
              </Alert>
            )}
          </CardContent>
        </Card>

        {canManage ? (
          <GitHubAppConfigDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            teamId={teamId}
            onSaved={async result => {
              await onChanged()
              setPostSave(result)
            }}
          />
        ) : null}

        {postSave ? (
          <GitHubAppPostSaveDialog
            open
            onOpenChange={open => {
              if (!open) setPostSave(null)
            }}
            teamId={teamId}
            webhookUrl={postSave.webhookUrl}
            webhookSecret={postSave.webhookSecret}
          />
        ) : null}
      </>
    )
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>GitHub App</CardTitle>
          <CardDescription>
            The App this team uses to reach GitHub.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-3 sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground text-sm">App slug</dt>
              <dd className="font-mono text-sm">{config.app_slug}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-sm">App ID</dt>
              <dd className="font-mono text-sm">{config.app_id}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-sm">Client ID</dt>
              <dd className="font-mono text-sm">{config.client_id}</dd>
            </div>
          </dl>

          <div className="flex flex-wrap gap-2">
            <SecretBadge label="Private key" present={config.has_private_key} />
            <SecretBadge
              label="Client secret"
              present={config.has_client_secret}
            />
            <SecretBadge
              label="Webhook secret"
              present={config.has_webhook_secret}
            />
          </div>

          {config.webhook_url ? (
            <CopyableValue label="Webhook URL" value={config.webhook_url} />
          ) : null}

          {!config.has_webhook_secret ? (
            <Alert variant="destructive">
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>No webhook secret set</AlertTitle>
              <AlertDescription>
                Installation events cannot be verified until one is configured.
              </AlertDescription>
            </Alert>
          ) : null}

          {canManage ? (
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                onClick={() => {
                  setDialogOpen(true)
                }}
              >
                Edit
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  void handleVerify()
                }}
                disabled={verifying}
              >
                {verifying ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Verifying…
                  </>
                ) : (
                  'Verify'
                )}
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setConfirmRotate(true)
                }}
              >
                Rotate webhook URL
              </Button>
              <Button
                variant="destructive"
                onClick={() => {
                  setConfirmDelete(true)
                }}
              >
                Remove
              </Button>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              Only a team owner or admin can change these settings.
            </p>
          )}
        </CardContent>
      </Card>

      {canManage ? (
        <>
          <GitHubAppConfigDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            teamId={teamId}
            config={config}
            onSaved={async result => {
              await onChanged()
              if (result.webhookSecret) setPostSave(result)
            }}
          />

          <ConfirmDialog
            open={confirmRotate}
            onOpenChange={setConfirmRotate}
            title="Rotate the webhook URL?"
            description="This mints a new routing token, so the webhook URL changes. GitHub keeps posting to the old URL until you update it there — deliveries stop until you do."
            confirmLabel="Rotate"
            onConfirm={handleRotate}
            loading={busy}
          />

          <ConfirmDialog
            open={confirmDelete}
            onOpenChange={setConfirmDelete}
            title="Remove this GitHub App?"
            description="The team's installations are disconnected and its repositories become unreachable from VibeXP. You will need to register an App and install it again to restore the integration."
            confirmLabel="Remove"
            variant="destructive"
            onConfirm={handleDelete}
            loading={busy}
          />
        </>
      ) : null}

      {postSave ? (
        <GitHubAppPostSaveDialog
          open
          onOpenChange={open => {
            if (!open) setPostSave(null)
          }}
          teamId={teamId}
          webhookUrl={postSave.webhookUrl}
          webhookSecret={postSave.webhookSecret}
        />
      ) : null}
    </>
  )
}

/**
 * Set / Not set, never the value. The secrets are write-only by construction —
 * the API has no field that could carry them back — so this is reporting what
 * the server said, not masking something we hold.
 */
function SecretBadge({ label, present }: { label: string; present?: boolean }) {
  return (
    <Badge variant={present ? 'secondary' : 'outline'}>
      {label}: {present ? 'Set' : 'Not set'}
    </Badge>
  )
}
