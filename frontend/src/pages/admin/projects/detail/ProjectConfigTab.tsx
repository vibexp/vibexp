import { Info } from 'lucide-react'
import { useCallback } from 'react'
import { Link } from 'react-router'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/time'
import {
  AdminConfigPanel,
  ConfigSection,
} from '@/pages/admin/teams/detail/AdminConfigPanel'
import { OnOffBadge } from '@/pages/admin/teams/detail/ConfigField'
import { useAdminTeamSection } from '@/pages/admin/teams/detail/useAdminTeamSection'
import { describeRule } from '@/pages/teams/settings/freshness/freshnessOptions'
import type { AdminFreshnessRule } from '@/services/adminService'
import { adminService } from '@/services/adminService'

/** One list of rules as plain text: sentence, threshold, status, last update. */
function RulesTable({
  rules,
  projectName,
  emptyMessage,
  testId,
}: Readonly<{
  rules: AdminFreshnessRule[]
  projectName: string
  emptyMessage: string
  testId: string
}>) {
  if (rules.length === 0) {
    return <p className="text-muted-foreground text-sm">{emptyMessage}</p>
  }
  return (
    <Card className="overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            <TableHead className="h-9 text-xs font-medium">Rule</TableHead>
            <TableHead className="h-9 text-xs font-medium">Threshold</TableHead>
            <TableHead className="h-9 text-xs font-medium">Status</TableHead>
            <TableHead className="h-9 text-xs font-medium">Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rules.map(rule => (
            <TableRow key={rule.id} data-testid={testId}>
              <TableCell className="py-3 text-sm">
                {describeRule(rule, projectName)}
              </TableCell>
              <TableCell className="py-3 text-sm tabular-nums">
                {`${String(rule.threshold_days)} day${rule.threshold_days === 1 ? '' : 's'}`}
              </TableCell>
              <TableCell className="py-3">
                <OnOffBadge
                  on={rule.enabled}
                  onLabel="Enabled"
                  offLabel="Disabled"
                />
              </TableCell>
              <TableCell className="py-3 text-sm">
                {formatDate(rule.updated_at)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

/**
 * The configuration that applies to a project (#1146), read-only. Projects
 * have no settings of their own, so this is the freshness rules that govern
 * it: its own, and the team-wide ones that also apply. The server splits the
 * two lists and never includes another project's rules; each list is still
 * filtered on its scope, so a server regression cannot show a rule that does
 * not govern this project.
 */
export function ProjectConfigTab({
  projectId,
  projectName,
  teamId,
}: Readonly<{ projectId: string; projectName: string; teamId: string }>) {
  const load = useCallback(
    () => adminService.getProjectConfig(projectId),
    [projectId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load project configuration'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load project configuration"
    >
      {data && (
        <>
          <Alert>
            <Info className="size-4" aria-hidden />
            <AlertDescription>
              Read-only view of the freshness rules that apply to this project.
            </AlertDescription>
          </Alert>
          <ConfigSection title="Rules for this project">
            <RulesTable
              rules={data.project_rules.filter(r => r.project_id === projectId)}
              projectName={projectName}
              emptyMessage="No rules target this project."
              testId="project-rule"
            />
          </ConfigSection>
          <ConfigSection title="Team-wide rules that also apply">
            <RulesTable
              rules={data.team_wide_rules.filter(r => r.project_id === null)}
              projectName={projectName}
              emptyMessage="No team-wide rules."
              testId="team-wide-rule"
            />
          </ConfigSection>
          <p className="text-sm">
            <Link
              to={`/admin/teams/${teamId}?tab=freshness`}
              className="hover:underline"
            >
              Team configuration →
            </Link>
          </p>
        </>
      )}
    </AdminConfigPanel>
  )
}
