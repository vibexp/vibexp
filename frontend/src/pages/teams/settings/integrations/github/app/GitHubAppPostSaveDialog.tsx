import { AlertTriangle, Loader2 } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/lib/toast'
import type { ValidateGitHubAppConfigResponse } from '@/services/githubAppConfigService'
import { githubAppConfigService } from '@/services/githubAppConfigService'

import { CopyableValue } from './CopyableValue'
import { describeValidationFailure } from './validationMessages'

export interface GitHubAppPostSaveDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  teamId: string
  webhookUrl: string
  /**
   * The generated webhook secret. Present only right after create or a secret
   * rotation — this dialog is the one and only time it is ever shown.
   */
  webhookSecret?: string
}

/**
 * The once-only disclosure step.
 *
 * The webhook URL exists only after the config row is saved, so an admin cannot
 * have configured the App's webhook before this point. This dialog is what
 * closes that loop: it hands over both values, says plainly that the secret will
 * not be shown again, and offers Verify so the round trip ends with proof rather
 * than hope.
 */
export function GitHubAppPostSaveDialog({
  open,
  onOpenChange,
  teamId,
  webhookUrl,
  webhookSecret,
}: GitHubAppPostSaveDialogProps) {
  const [verifying, setVerifying] = useState(false)
  const [result, setResult] = useState<ValidateGitHubAppConfigResponse | null>(
    null
  )

  const handleVerify = async () => {
    setVerifying(true)
    try {
      const response = await githubAppConfigService.validateAppConfig(teamId)
      setResult(response)
      if (response.is_valid) {
        toast.success('GitHub App verified')
      }
    } catch {
      toast.error('Could not verify the GitHub App')
    } finally {
      setVerifying(false)
    }
  }

  const failure =
    result && !result.is_valid
      ? describeValidationFailure(result.details?.error_details, result.message)
      : null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Finish setup on GitHub</DialogTitle>
          <DialogDescription>
            Paste these into your GitHub App’s <strong>Webhook</strong>{' '}
            settings, then verify. Until you do, VibeXP receives no installation
            events.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <CopyableValue label="Webhook URL" value={webhookUrl} />

          {webhookSecret ? (
            <>
              <CopyableValue
                label="Webhook secret"
                value={webhookSecret}
                multiline
              />
              <Alert variant="destructive">
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle>This secret is shown once</AlertTitle>
                <AlertDescription>
                  VibeXP never displays it again — later reads only tell you
                  whether one is set. If you lose it before pasting it into
                  GitHub, rotate the secret to get a new one.
                </AlertDescription>
              </Alert>
            </>
          ) : null}

          {result?.is_valid ? (
            <Alert>
              <AlertTitle>Verified</AlertTitle>
              <AlertDescription>{result.message}</AlertDescription>
            </Alert>
          ) : null}

          {failure ? (
            <Alert variant="destructive">
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>{failure.title}</AlertTitle>
              <AlertDescription>{failure.description}</AlertDescription>
            </Alert>
          ) : null}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            Close
          </Button>
          <Button
            type="button"
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
