import { PageHeader } from '@/components/PageHeader'
import {
  type SettingItem,
  SettingSection,
} from '@/components/settings/SettingsGrid'
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
  const items: SettingItem[] = []

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
