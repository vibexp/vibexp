import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { GitHubIcon } from '@/components/icons/GitHubIcon'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import type {
  GitHubInstallationStatus,
  GitHubInstallCallbackRequest,
  GitHubRepository,
} from '@/services/githubIntegrationService'
import { githubIntegrationService } from '@/services/githubIntegrationService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { ApiError } from '@/types/errors'
import { safeRedirect } from '@/utils/urlValidation'

import { GitHubConnectionCard } from './GitHubConnectionCard'
import { GitHubInstallModal } from './GitHubInstallModal'
import { GitHubRepositoryList } from './GitHubRepositoryList'
import { GitHubUninstallStepDialog } from './GitHubUninstallStepDialog'
import { useGitHubDisconnect } from './useGitHubDisconnect'

const REPOS_PER_SERVER_PAGE = 100

export function GitHubIntegration() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { currentTeam } = useTeam()
  const { trackEvent } = useAnalytics()
  const { handleError } = useErrorHandler()

  const [status, setStatus] = useState<GitHubInstallationStatus | null>(null)
  const [repositories, setRepositories] = useState<GitHubRepository[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isLoadingRepos, setIsLoadingRepos] = useState(false)
  const [showInstallModal, setShowInstallModal] = useState(false)
  const [totalRepos, setTotalRepos] = useState(0)
  const [isLaunching, setIsLaunching] = useState(false)
  const [hasMore, setHasMore] = useState<boolean>(false)
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false)
  const [serverPage, setServerPage] = useState<number>(1)
  const abortControllerRef = useRef<AbortController | null>(null)

  const loadStatus = useCallback(async () => {
    if (!currentTeam) return
    try {
      setIsLoading(true)
      const statusData = await githubIntegrationService.getStatus(
        currentTeam.id
      )
      setStatus(statusData)
    } catch (error) {
      handleError(error, 'Failed to load GitHub integration status')
    } finally {
      setIsLoading(false)
    }
  }, [currentTeam, handleError])

  const {
    showDisconnectDialog,
    setShowDisconnectDialog,
    isDisconnecting,
    pendingUninstallId,
    pendingAccountType,
    showUninstallStep,
    handleDisconnect,
    handleUninstallStepClose,
  } = useGitHubDisconnect({
    teamId: currentTeam?.id,
    status,
    repositories,
    onDisconnected: loadStatus,
    onResetRepositories: () => {
      setRepositories([])
      setTotalRepos(0)
    },
  })

  const loadRepositories = useCallback(async () => {
    if (!currentTeam) return

    try {
      setIsLoadingRepos(true)
      const response = await githubIntegrationService.getRepositories(
        currentTeam.id,
        1,
        abortControllerRef.current?.signal
      )

      setRepositories(response.repositories)
      setTotalRepos(response.total_count)
      setServerPage(1)
      const totalPages = Math.ceil(response.total_count / REPOS_PER_SERVER_PAGE)
      setHasMore(totalPages > 1)

      trackEvent({
        event: ANALYTICS_EVENTS.GITHUB_REPOSITORIES_VIEWED,
        properties: {
          total_count: response.total_count,
          pages_fetched: 1,
          action_context: 'view',
        },
      })
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return
      }
      handleError(error, 'Failed to load repositories')
    } finally {
      setIsLoadingRepos(false)
    }
  }, [currentTeam, handleError, trackEvent])

  const loadMoreRepositories = useCallback(async () => {
    if (!currentTeam || isLoadingMore) return

    setIsLoadingMore(true)
    try {
      const nextPage = serverPage + 1
      const response = await githubIntegrationService.getRepositories(
        currentTeam.id,
        nextPage,
        abortControllerRef.current?.signal
      )

      setRepositories(prev => [...prev, ...response.repositories])
      setServerPage(nextPage)
      setTotalRepos(response.total_count)
      const totalLoaded = nextPage * REPOS_PER_SERVER_PAGE
      setHasMore(totalLoaded < response.total_count)
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return
      }
      handleError(error, 'Failed to load more repositories')
    } finally {
      setIsLoadingMore(false)
    }
  }, [currentTeam, serverPage, isLoadingMore, handleError])

  const handleCallback = useCallback(
    async (data: GitHubInstallCallbackRequest) => {
      if (!currentTeam) return

      try {
        const response = await githubIntegrationService.handleCallback(
          currentTeam.id,
          data
        )

        void navigate('/settings/integrations/github', { replace: true })
        await loadStatus()
        if (response.reconnected) {
          toast.success('Reconnected to existing GitHub installation')
        } else {
          toast.success('GitHub integration connected successfully')
        }

        trackEvent({
          event: ANALYTICS_EVENTS.GITHUB_CONNECTED,
          properties: {
            installation_id: data.installation_id,
            action_context: 'connect',
          },
        })
      } catch (error) {
        if (
          error instanceof ApiError &&
          error.code === 'installation_already_connected'
        ) {
          handleError(
            error,
            'This GitHub organization is already connected to another team. Each GitHub org/account can only be connected to one team.'
          )
        } else {
          handleError(error, 'Failed to complete GitHub installation')
        }
        void navigate('/settings/integrations/github', { replace: true })
      }
    },
    [currentTeam, handleError, loadStatus, navigate, trackEvent]
  )

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.GITHUB_INTEGRATION_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  useEffect(() => {
    if (currentTeam) {
      void loadStatus()
    }
  }, [currentTeam, loadStatus])

  useEffect(() => {
    abortControllerRef.current = new AbortController()
    if (status?.installed && !status.suspended && currentTeam) {
      void loadRepositories()
    }
    const controller = abortControllerRef.current
    return () => {
      controller.abort()
    }
  }, [status?.installed, status?.suspended, currentTeam, loadRepositories])

  const installationIdStr = searchParams.get('installation_id')
  const setupAction = searchParams.get('setup_action')
  const state = searchParams.get('state')

  useEffect(() => {
    if (installationIdStr && setupAction && state && currentTeam) {
      const installationId = Number.parseInt(installationIdStr, 10)

      if (Number.isNaN(installationId) || installationId <= 0) {
        handleError(
          new Error(`Invalid installation ID: ${installationIdStr}`),
          'Failed to complete GitHub installation'
        )
        return
      }

      void handleCallback({
        installation_id: installationId,
        setup_action: setupAction,
        state,
      })
    }
  }, [
    installationIdStr,
    setupAction,
    state,
    currentTeam,
    handleCallback,
    handleError,
  ])

  const handleConnect = () => {
    setShowInstallModal(true)
  }

  const handleLaunchInstall = async () => {
    if (!currentTeam) return
    try {
      setIsLaunching(true)
      const response = await githubIntegrationService.getInstallUrl(
        currentTeam.id
      )
      safeRedirect(response.install_url, ['github.com'])
    } catch (error) {
      handleError(error, 'Failed to get GitHub install URL')
      setIsLaunching(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="GitHub Integration"
        description="Connect GitHub repositories to your team workspace."
        actions={
          !status?.installed && (
            <Button onClick={handleConnect}>
              <GitHubIcon className="mr-2 size-4" />
              Connect GitHub
            </Button>
          )
        }
      />

      <GitHubConnectionCard
        status={status}
        onDisconnect={() => {
          setShowDisconnectDialog(true)
        }}
        isLoading={isLoading}
      />

      {status?.installed && !status.suspended && (
        <div className="space-y-3">
          <h2 className="text-lg font-semibold">Accessible Repositories</h2>
          <GitHubRepositoryList
            repositories={repositories}
            isLoading={isLoadingRepos}
            totalCount={totalRepos}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onLoadMore={() => {
              void loadMoreRepositories()
            }}
          />
        </div>
      )}

      <GitHubInstallModal
        isOpen={showInstallModal}
        onClose={() => {
          setShowInstallModal(false)
        }}
        onLaunch={() => {
          void handleLaunchInstall()
        }}
        isLaunching={isLaunching}
      />

      <ConfirmDialog
        open={showDisconnectDialog}
        onOpenChange={setShowDisconnectDialog}
        title="Disconnect GitHub Integration"
        description="Are you sure you want to disconnect GitHub? You will lose access to all connected repositories and GitHub-powered features."
        confirmLabel="Disconnect"
        variant="destructive"
        loading={isDisconnecting}
        onConfirm={handleDisconnect}
      />

      <GitHubUninstallStepDialog
        isOpen={showUninstallStep}
        installationId={pendingUninstallId}
        onClose={handleUninstallStepClose}
        accountType={pendingAccountType}
      />
    </div>
  )
}
