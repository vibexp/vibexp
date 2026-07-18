import { Info } from 'lucide-react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface GitHubInstallModalProps {
  isOpen: boolean
  onClose: () => void
  onLaunch: () => void
  isLaunching: boolean
}

export function GitHubInstallModal({
  isOpen,
  onClose,
  onLaunch,
  isLaunching,
}: Readonly<GitHubInstallModalProps>) {
  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Connect GitHub</DialogTitle>
          <DialogDescription>
            Follow these steps to connect your GitHub account.
          </DialogDescription>
        </DialogHeader>

        <ol className="text-muted-foreground list-inside list-decimal space-y-2 text-sm">
          <li>
            Click the &quot;Install GitHub App&quot; button below to open GitHub
          </li>
          <li>
            Select the organization or account where you want to install the
            VibeXP app
          </li>
          <li>
            Choose which repositories to grant access to (all or selected)
          </li>
          <li>
            Click &quot;Install &amp; Authorize&quot; on GitHub to complete the
            setup
          </li>
          <li>You&apos;ll be redirected back to VibeXP automatically</li>
        </ol>

        <Alert>
          <Info className="size-4" />
          <AlertDescription>
            You can modify repository access and permissions at any time from
            your GitHub settings.
          </AlertDescription>
        </Alert>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={onClose} disabled={isLaunching}>
            Cancel
          </Button>
          <Button onClick={onLaunch} disabled={isLaunching}>
            {isLaunching ? 'Launching…' : 'Install GitHub App'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
