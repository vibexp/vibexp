import { Activity, Bell, Key, Users } from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import {
  type SettingItem,
  SettingSection,
} from '@/components/settings/SettingsGrid'
import { useTeam } from '@/contexts/TeamContext'

/**
 * Genuinely personal settings — every destination here is user-scoped at the
 * API layer (`/activities`, `/preferences`, `/settings/api-keys`).
 *
 * Everything team-scoped moved out under `/teams/:id/**` during epic #536:
 * teams (#538), search / model providers / embedding providers / artifact types
 * (#540), GitHub integration (#541) and projects (#542). Adding a card here
 * whose destination acts on a team is the regression this page's test guards
 * against — it asserts the exact set.
 */
const ACCOUNT: SettingItem[] = [
  {
    title: 'Activities',
    description:
      'Monitor and track your account activities and security events.',
    icon: Activity,
    href: '/settings/activities',
  },
  {
    title: 'Notification Preferences',
    description: 'Manage your email notification settings and preferences.',
    icon: Bell,
    href: '/settings/notifications',
  },
  {
    title: 'API Keys',
    description:
      'Create and manage API keys for programmatic access to your account.',
    icon: Key,
    href: '/settings/api-keys',
  },
]

export function Settings() {
  const { currentTeam, isLoading } = useTeam()

  // Named after the team so the destination is unambiguous — this is the one
  // card on the page that leaves the personal scope.
  const teamPointer: SettingItem[] =
    !isLoading && currentTeam
      ? [
          {
            title: 'Team settings',
            description: `Configure search, providers, integrations and artifact types for ${currentTeam.name}.`,
            icon: Users,
            href: `/teams/${currentTeam.id}/settings`,
          },
        ]
      : []

  return (
    <div className="space-y-8">
      <PageHeader
        title="Settings"
        description="Manage your personal account settings. Team configuration lives under each team."
      />
      <SettingSection title="Account" items={ACCOUNT} />
      {/* Hidden while the team list hydrates and when there is no team at all:
          a pointer built from a null team would link to /teams/undefined. */}
      {teamPointer.length > 0 && (
        <SettingSection title="Team" items={teamPointer} />
      )}
    </div>
  )
}
