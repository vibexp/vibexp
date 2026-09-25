import { useCallback } from 'react'

import { AI_SUMMARY_STYLES } from '@/pages/teams/settings/model-providers/aiSummaryForm'
import type { AdminTeamAISummaryConfig } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import { ConfigField, ConfigGrid, OnOffBadge, SourceBadge } from './ConfigField'
import { useAdminTeamSection } from './useAdminTeamSection'

/**
 * The provider name, or why there is none: no id means "no provider selected",
 * an id without a name means the selected provider no longer exists.
 */
function providerLabel(config: AdminTeamAISummaryConfig): string {
  if (config.model_provider_name) return config.model_provider_name
  return config.values.model_provider_id ? 'Provider removed' : 'No provider'
}

/** AI summary settings in effect for the team (read-only). */
export function TeamAISummaryConfigTab({
  teamId,
}: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamAISummaryConfig(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load AI summary settings'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load AI summary settings"
    >
      {data && (
        <ConfigSection
          title="AI summary"
          aside={<SourceBadge source={data.source} />}
        >
          <ConfigGrid>
            <ConfigField label="Enabled">
              <OnOffBadge on={data.values.enabled} />
            </ConfigField>
            <ConfigField label="Provider">{providerLabel(data)}</ConfigField>
            <ConfigField label="Availability">
              <OnOffBadge
                on={data.available}
                onLabel="Available"
                offLabel="No model provider"
              />
            </ConfigField>
            <ConfigField label="Top N">
              {data.values.top_n} (max {data.max_top_n})
            </ConfigField>
            <ConfigField label="Style">
              {AI_SUMMARY_STYLES.find(s => s.id === data.values.style)?.label ??
                data.values.style}
            </ConfigField>
            <ConfigField label="Max output tokens">
              {data.values.max_output_tokens} (max{' '}
              {data.max_output_tokens_ceiling})
            </ConfigField>
          </ConfigGrid>
        </ConfigSection>
      )}
    </AdminConfigPanel>
  )
}
