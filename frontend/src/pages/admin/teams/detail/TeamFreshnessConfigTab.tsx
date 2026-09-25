import { useCallback } from 'react'
import { Link } from 'react-router'

import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  describeInterval,
  describeRule,
} from '@/pages/teams/settings/freshness/freshnessOptions'
import type { AdminFreshnessRule } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import { ConfigField, ConfigGrid, OnOffBadge, SourceBadge } from './ConfigField'
import { shortId } from './teamConfigFormat'
import { useAdminTeamSection } from './useAdminTeamSection'

function RulesTable({ rules }: Readonly<{ rules: AdminFreshnessRule[] }>) {
  if (rules.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        This team has no freshness rules.
      </p>
    )
  }
  return (
    <Card className="overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            <TableHead className="h-9 text-xs font-medium">Rule</TableHead>
            <TableHead className="h-9 text-xs font-medium">Scope</TableHead>
            <TableHead className="h-9 text-xs font-medium">Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rules.map(rule => (
            <TableRow key={rule.id} data-testid="freshness-rule">
              <TableCell className="py-3 text-sm">
                {describeRule(
                  rule,
                  rule.project_id
                    ? `project ${shortId(rule.project_id)}`
                    : undefined
                )}
              </TableCell>
              <TableCell className="py-3 text-sm">
                {rule.project_id ? (
                  <Link
                    to={`/admin/projects/${rule.project_id}`}
                    className="font-mono hover:underline"
                  >
                    {shortId(rule.project_id)}
                  </Link>
                ) : (
                  'Team-wide'
                )}
              </TableCell>
              <TableCell className="py-3">
                <OnOffBadge
                  on={rule.enabled}
                  onLabel="Enabled"
                  offLabel="Disabled"
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

/** Freshness evaluation settings and rules for the team (read-only). */
export function TeamFreshnessConfigTab({
  teamId,
}: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamFreshnessConfig(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load freshness settings'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load freshness settings"
    >
      {data && (
        <>
          <ConfigSection
            title="Freshness evaluation"
            aside={<SourceBadge source={data.source} />}
          >
            <ConfigGrid>
              <ConfigField label="Interval">
                {describeInterval(data.values.interval_seconds)}
              </ConfigField>
              <ConfigField label="Reversibility">
                <OnOffBadge on={data.values.reversibility_enabled} />
              </ConfigField>
            </ConfigGrid>
          </ConfigSection>
          <ConfigSection title="Rules">
            <RulesTable rules={data.rules} />
          </ConfigSection>
        </>
      )}
    </AdminConfigPanel>
  )
}
