import { useCallback, useEffect, useState } from 'react'

import { useErrorHandler } from '@/hooks/useErrorHandler'
import type { GitHubAppConfigResponse } from '@/services/githubAppConfigService'
import {
  githubAppConfigService,
  isGitHubAppNotConfigured,
} from '@/services/githubAppConfigService'

export interface UseGitHubAppConfigResult {
  /**
   * The team's registration, `null` once loaded and there is none, `undefined`
   * while still loading.
   *
   * The three-state value is what keeps the install affordance from flashing in
   * before the answer is known — "no App" and "not asked yet" must not render
   * the same way.
   */
  appConfig: GitHubAppConfigResponse | null | undefined
  reload: () => Promise<void>
}

/**
 * Loads the team's own GitHub App configuration (#484).
 *
 * A team with no App comes back as a 409, which is the documented empty state
 * rather than a failure — reporting it as an error would put a red toast in
 * front of every team that has simply not set one up yet.
 */
export function useGitHubAppConfig(
  teamId: string | undefined
): UseGitHubAppConfigResult {
  const { handleError } = useErrorHandler()
  const [appConfig, setAppConfig] = useState<
    GitHubAppConfigResponse | null | undefined
  >(undefined)

  const reload = useCallback(async () => {
    if (!teamId) return
    try {
      setAppConfig(await githubAppConfigService.getAppConfig(teamId))
    } catch (error) {
      if (isGitHubAppNotConfigured(error)) {
        setAppConfig(null)
        return
      }
      handleError(error, 'Failed to load the GitHub App configuration')
      setAppConfig(null)
    }
  }, [teamId, handleError])

  useEffect(() => {
    void reload()
  }, [reload])

  return { appConfig, reload }
}
