import { PageHeader } from '@/components/PageHeader'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Team } from '@/services/teamService'

import { FreshnessAnalytics } from './FreshnessAnalytics'
import { FreshnessAudit } from './FreshnessAudit'
import { FreshnessRulesTab } from './FreshnessRulesTab'

/**
 * The Resource Freshness page (epic #726): one destination behind the team
 * settings card, split into three tabs.
 *
 *  - **Settings** (#736) — evaluation cadence, reversibility, and the rules.
 *    Writes require `team.settings.update`; the tab gates them itself.
 *  - **Analytics** (#737) — how staleness is trending and where it sits.
 *  - **Audit** (#737) — every mark and clear, and why.
 *
 * Analytics and Audit are **readable by every member**, including those who
 * cannot edit anything: the engine writes to everyone's resources, so everyone
 * is entitled to see what it did. Only the Settings tab's write controls are
 * permission-gated, which is why the gating lives in that tab rather than here.
 *
 * The tabs are lazy by construction — Radix unmounts inactive `TabsContent`, so
 * the analytics and audit requests only fire once their tab is opened, and the
 * page costs one settings + one rules call on arrival.
 */
export function FreshnessSettings({ team }: Readonly<{ team: Team }>) {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Resource Freshness"
        description="Flag resources your team has stopped using, so the workspace reflects what you actually rely on."
      />

      <Tabs defaultValue="settings" className="space-y-6">
        <TabsList>
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="analytics">Analytics</TabsTrigger>
          <TabsTrigger value="audit">Audit</TabsTrigger>
        </TabsList>

        <TabsContent value="settings">
          <FreshnessRulesTab team={team} />
        </TabsContent>

        <TabsContent value="analytics">
          <FreshnessAnalytics teamId={team.id} />
        </TabsContent>

        <TabsContent value="audit">
          <FreshnessAudit teamId={team.id} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
