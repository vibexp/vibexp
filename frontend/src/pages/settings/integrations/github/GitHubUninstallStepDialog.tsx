import { ExternalLink, Info } from 'lucide-react'

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

interface GitHubUninstallStepDialogProps {
  isOpen: boolean
  installationId: number | null
  onClose: () => void
  accountType?: string
}

export function GitHubUninstallStepDialog({
  isOpen,
  installationId,
  onClose,
  accountType,
}: Readonly<GitHubUninstallStepDialogProps>) {
  const uninstallUrl =
    installationId !== null
      ? `https://github.com/settings/installations/${String(installationId)}`
      : 'https://github.com/settings/installations'

  const isOrg = accountType === 'Organization'

  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>GitHub disconnected — one more step</DialogTitle>
          <DialogDescription>
            Your GitHub integration has been disconnected from VibeXP. However,
            the GitHub App still has access to your repositories. To fully
            revoke access, you need to uninstall it from GitHub.
          </DialogDescription>
        </DialogHeader>

        <Alert>
          <Info className="size-4" />
          <AlertTitle>Why this matters</AlertTitle>
          <AlertDescription>
            Until the GitHub App is uninstalled, it retains read access to the
            repositories you granted. Completing the uninstall ensures no
            residual permissions remain.
          </AlertDescription>
        </Alert>

        {isOrg && (
          <Alert>
            <Info className="size-4" />
            <AlertTitle>Organization installation</AlertTitle>
            <AlertDescription>
              This installation may be owned by an organization — an org admin
              may need to uninstall it.
            </AlertDescription>
          </Alert>
        )}

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={onClose}>
            Skip for now
          </Button>
          <Button asChild>
            <a
              href={uninstallUrl}
              target="_blank"
              rel="noopener noreferrer"
              onClick={onClose}
            >
              <ExternalLink className="mr-2 size-4" />
              Uninstall from GitHub
            </a>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
