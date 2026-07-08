import { useState } from 'react'

import { useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import type {
  GitHubInstallationStatus,
  GitHubRepository,
} from '@/services/githubIntegrationService'
import { githubIntegrationService } from '@/services/githubIntegrationService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

interface UseGitHubDisconnectParams {
  teamId: string | undefined
  status: GitHubInstallationStatus | null
  repositories: GitHubRepository[]
  onDisconnected: () => Promise<void>
  onResetRepositories: () => void
}

export function useGitHubDisconnect({
  teamId,
  status,
  repositories,
  onDisconnected,
  onResetRepositories,
}: UseGitHubDisconnectParams) {
  const { trackEvent } = useAnalytics()
  const { handleError } = useErrorHandler()

  const [showDisconnectDialog, setShowDisconnectDialog] = useState(false)
  const [isDisconnecting, setIsDisconnecting] = useState(false)
  const [pendingUninstallId, setPendingUninstallId] = useState<number | null>(
    null
  )
  const [pendingAccountType, setPendingAccountType] = useState<
    string | undefined
  >(undefined)
  const [showUninstallStep, setShowUninstallStep] = useState(false)

  const handleDisconnect = async () => {
    if (!teamId) return
    const installationId = status?.installation_id ?? null
    const accountType =
      repositories.length > 0 ? repositories[0].owner.type : undefined

    try {
      setIsDisconnecting(true)
      await githubIntegrationService.disconnect(teamId)
      setShowDisconnectDialog(false)
      await onDisconnected()
      onResetRepositories()
      toast.success('GitHub integration disconnected successfully')
      setPendingUninstallId(installationId)
      setPendingAccountType(accountType)
      setShowUninstallStep(true)
      trackEvent({
        event: ANALYTICS_EVENTS.GITHUB_DISCONNECTED,
        properties: { action_context: 'disconnect' },
      })
    } catch (error) {
      handleError(error, 'Failed to disconnect GitHub integration')
    } finally {
      setIsDisconnecting(false)
    }
  }

  const handleUninstallStepClose = () => {
    setShowUninstallStep(false)
    setPendingUninstallId(null)
    setPendingAccountType(undefined)
  }

  return {
    showDisconnectDialog,
    setShowDisconnectDialog,
    isDisconnecting,
    pendingUninstallId,
    pendingAccountType,
    showUninstallStep,
    handleDisconnect,
    handleUninstallStepClose,
  }
}
