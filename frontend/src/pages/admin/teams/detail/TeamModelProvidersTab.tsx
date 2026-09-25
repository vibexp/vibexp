import { useCallback } from 'react'

import { adminService } from '@/services/adminService'

import { AdminConfigPanel } from './AdminConfigPanel'
import { ProviderTable } from './ProviderTable'
import { useAdminTeamSection } from './useAdminTeamSection'

/**
 * The team's own model providers (read-only). Team rows only: an instance
 * fallback never counts as configured, so an empty list says so plainly.
 */
export function TeamModelProvidersTab({
  teamId,
}: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamModelProviders(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load model providers'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load model providers"
      empty={data?.providers.length === 0}
      emptyMessage="No model providers configured for this team."
    >
      {data && <ProviderTable providers={data.providers} />}
    </AdminConfigPanel>
  )
}
