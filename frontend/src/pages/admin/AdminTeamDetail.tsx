import { useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatDate } from '@/lib/time'
import { AdminDetailScaffold } from '@/pages/admin/AdminDetailScaffold'
import { TeamAISummaryConfigTab } from '@/pages/admin/teams/detail/TeamAISummaryConfigTab'
import { TeamArtifactTypesTab } from '@/pages/admin/teams/detail/TeamArtifactTypesTab'
import { TeamEmailConfigTab } from '@/pages/admin/teams/detail/TeamEmailConfigTab'
import { TeamEmbeddingProvidersTab } from '@/pages/admin/teams/detail/TeamEmbeddingProvidersTab'
import { TeamFreshnessConfigTab } from '@/pages/admin/teams/detail/TeamFreshnessConfigTab'
import { TeamGitHubConfigTab } from '@/pages/admin/teams/detail/TeamGitHubConfigTab'
import { TeamMembersTab } from '@/pages/admin/teams/detail/TeamMembersTab'
import { TeamModelProvidersTab } from '@/pages/admin/teams/detail/TeamModelProvidersTab'
import { TeamSearchConfigTab } from '@/pages/admin/teams/detail/TeamSearchConfigTab'
import { TeamSettingsAuditTab } from '@/pages/admin/teams/detail/TeamSettingsAuditTab'
import type { AdminTeamDetail as AdminTeamDetailType } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const TABS = [
  'members',
  'search',
  'ai-summary',
  'freshness',
  'model-providers',
  'embedding-providers',
  'email',
  'github',
  'artifact-types',
  'settings-audit',
] as const
type TeamDetailTab = (typeof TABS)[number]

const TAB_LABELS: Record<TeamDetailTab, string> = {
  members: 'Members',
  search: 'Search',
  'ai-summary': 'AI summary',
  freshness: 'Freshness',
  'model-providers': 'Model providers',
  'embedding-providers': 'Embedding providers',
  email: 'Email',
  github: 'GitHub',
  'artifact-types': 'Artifact types',
  'settings-audit': 'Settings audit',
}

function isTab(value: string | null): value is TeamDetailTab {
  return TABS.includes(value as TeamDetailTab)
}

/**
 * Instance team detail — owner and summary (#316) above a Members tab and
 * nine read-only configuration tabs (#1142). The active tab lives in `?tab=`
 * so it survives a reload and can be linked to; Radix unmounts inactive tabs,
 * so each configuration tab fetches only when it is opened.
 */
export function AdminTeamDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const tab: TeamDetailTab = isTab(requestedTab) ? requestedTab : 'members'
  const setTab = (value: string) => {
    setSearchParams(
      prev => {
        const next = new URLSearchParams(prev)
        next.set('tab', value)
        return next
      },
      { replace: true }
    )
  }
  const [team, setTeam] = useState<AdminTeamDetailType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getTeam(id)
      .then(result => {
        if (!cancelled) setTeam(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, 'Failed to load team'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id])

  return (
    <AdminDetailScaffold
      backTo="/admin/teams"
      backLabel="Back to teams"
      loading={loading}
      error={error}
      errorTitle="Failed to load team"
    >
      {team && (
        <>
          <PageHeader title={team.name} />
          <Card>
            <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-3">
              <div>
                <p className="text-muted-foreground text-xs">Owner</p>
                <p className="text-sm">{team.owner.email}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Created</p>
                <p className="text-sm">{formatDate(team.created_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Members</p>
                <p className="text-sm tabular-nums">{team.members.length}</p>
              </div>
            </CardContent>
          </Card>

          <Tabs value={tab} onValueChange={setTab} className="space-y-6">
            <TabsList className="h-auto w-full justify-start overflow-x-auto">
              {TABS.map(value => (
                <TabsTrigger key={value} value={value} className="shrink-0">
                  {TAB_LABELS[value]}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value="members">
              <TeamMembersTab members={team.members} />
            </TabsContent>
            <TabsContent value="search">
              <TeamSearchConfigTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="ai-summary">
              <TeamAISummaryConfigTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="freshness">
              <TeamFreshnessConfigTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="model-providers">
              <TeamModelProvidersTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="embedding-providers">
              <TeamEmbeddingProvidersTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="email">
              <TeamEmailConfigTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="github">
              <TeamGitHubConfigTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="artifact-types">
              <TeamArtifactTypesTab teamId={team.id} />
            </TabsContent>
            <TabsContent value="settings-audit">
              <TeamSettingsAuditTab teamId={team.id} />
            </TabsContent>
          </Tabs>
        </>
      )}
    </AdminDetailScaffold>
  )
}
