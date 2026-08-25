import { PageHeader } from '@/components/PageHeader'
import { SettingSection } from '@/components/settings/SettingsGrid'
import { usePermissions } from '@/hooks/usePermissions'
import { teamSettingsCardsFor } from '@/pages/teams/settings/team-settings-cards'
import type { Team } from '@/services/teamService'

/**
 * Team settings hub at `/teams/:id/settings` (#539).
 *
 * Rendered with `SettingSection` — the component #537 extracted out of
 * `pages/settings/Settings.tsx` — so this hub and the personal one are
 * identical by construction rather than by two copies of the same markup
 * staying in sync. Adding a card here is a data change, never a markup one.
 *
 * It ships with **no cards**: every team-scoped settings page still lives under
 * `/settings/**` until #540 (search, model providers, embedding providers,
 * artifact types) and #541 (GitHub integration) relocate them. That empty state
 * is expected at this point in epic #536, not a defect.
 */
export function TeamSettings({ team }: Readonly<{ team: Team }>) {
  // The team is the one `TeamScopeLayout` resolved from the URL, so the
  // permissions must be read off *it*. The ambient team is a different team on
  // a cold deep-link, which would gate this hub on the wrong team's role.
  const { can } = usePermissions(team)
  const items = teamSettingsCardsFor(team.id).filter(
    item => !item.permission || can(item.permission)
  )

  return (
    <div className="space-y-8">
      <PageHeader
        title="Team settings"
        description={`Manage settings and configurations for ${team.name}.`}
      />
      {items.length > 0 ? (
        <SettingSection title="Configuration" items={items} />
      ) : (
        <p className="text-muted-foreground text-sm">
          No team settings have moved here yet. Team-scoped configuration is
          still under Settings while it is relocated.
        </p>
      )}
    </div>
  )
}
