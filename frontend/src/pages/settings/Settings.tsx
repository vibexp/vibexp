import {
  Activity,
  Bell,
  Bot,
  Cpu,
  FolderKanban,
  Key,
  Shapes,
  SlidersHorizontal,
  Users,
} from 'lucide-react'

import { GitHubIcon } from '@/components/icons/GitHubIcon'
import { PageHeader } from '@/components/PageHeader'
import {
  type SettingItem,
  SettingSection,
} from '@/components/settings/SettingsGrid'

const GENERAL: SettingItem[] = [
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
]

const INTEGRATION: SettingItem[] = [
  {
    title: 'API Keys',
    description:
      'Create and manage API keys for programmatic access to your account.',
    icon: Key,
    href: '/settings/api-keys',
  },
  {
    title: 'Embedding Providers',
    description:
      'Configure embedding vector providers for your AI applications.',
    icon: Cpu,
    href: '/settings/embedding-providers',
  },
  {
    title: 'Model Providers',
    description:
      'Configure OpenAI-compatible LLM providers for your AI applications.',
    icon: Bot,
    href: '/settings/model-providers',
  },
  {
    title: 'GitHub Integration',
    description: 'Connect GitHub repositories to your team workspace.',
    icon: GitHubIcon,
    href: '/settings/integrations/github',
  },
]

const CUSTOMIZATION: SettingItem[] = [
  {
    title: 'Artifact Types',
    description: 'Create and manage custom categories for your artifacts.',
    icon: Shapes,
    href: '/settings/customization',
  },
  {
    title: 'Search Settings',
    description: 'Choose how search results are ranked for your team.',
    icon: SlidersHorizontal,
    href: '/settings/search',
  },
]

const COLLABORATION: SettingItem[] = [
  {
    title: 'Teams',
    description: 'Manage your team memberships and collaborate with others.',
    icon: Users,
    href: '/settings/teams',
  },
  {
    title: 'Projects',
    description:
      'Organize your artifacts, blueprints, and resources into projects.',
    icon: FolderKanban,
    href: '/settings/projects',
  },
]

export function Settings() {
  return (
    <div className="space-y-8">
      <PageHeader
        title="Settings"
        description="Manage your account settings and configurations."
      />
      <SettingSection title="General" items={GENERAL} />
      <SettingSection title="Integration" items={INTEGRATION} />
      <SettingSection title="Customization" items={CUSTOMIZATION} />
      <SettingSection title="Collaboration" items={COLLABORATION} />
    </div>
  )
}
