import { useCallback } from 'react'

import type { AdminSearchValues } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import { ConfigField, ConfigGrid, OnOffBadge, SourceBadge } from './ConfigField'
import { useAdminTeamSection } from './useAdminTeamSection'

function SearchValues({ values }: Readonly<{ values: AdminSearchValues }>) {
  return (
    <ConfigGrid>
      <ConfigField label="Recency ranking">
        <OnOffBadge on={values.recency_ranking_enabled} />
      </ConfigField>
      <ConfigField label="Relevance weight">
        {values.rank_weight_relevance}
      </ConfigField>
      <ConfigField label="Created weight">
        {values.rank_weight_created}
      </ConfigField>
      <ConfigField label="Updated weight">
        {values.rank_weight_updated}
      </ConfigField>
      <ConfigField label="Half-life (days)">
        {values.rank_half_life_days}
      </ConfigField>
    </ConfigGrid>
  )
}

/** Search ranking settings in effect for the team (read-only). */
export function TeamSearchConfigTab({ teamId }: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamSearchConfig(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load search settings'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load search settings"
    >
      {data && (
        <>
          <ConfigSection
            title="Search ranking"
            aside={<SourceBadge source={data.source} />}
          >
            <SearchValues values={data.values} />
            <ConfigGrid>
              <ConfigField label="Rank candidate cap (instance)">
                {data.rank_candidate_cap}
              </ConfigField>
            </ConfigGrid>
          </ConfigSection>
          {data.source === 'team' && (
            <ConfigSection title="Instance defaults">
              <SearchValues values={data.instance_defaults} />
            </ConfigSection>
          )}
        </>
      )}
    </AdminConfigPanel>
  )
}
