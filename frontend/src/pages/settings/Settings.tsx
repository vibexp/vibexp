import {
  Activity,
  ArrowRight,
  Bell,
  Bot,
  Cpu,
  FolderKanban,
  Key,
  Shapes,
  Users,
} from 'lucide-react'
import type { ComponentType } from 'react'
import { useNavigate } from 'react-router-dom'

import { GitHubIcon } from '@/components/icons/GitHubIcon'
import { PageHeader } from '@/components/PageHeader'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

interface SettingItem {
  title: string
  description: string
  icon: ComponentType<{ className?: string }>
  href: string
}

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

function SettingSection({
  title,
  items,
}: Readonly<{
  title: string
  items: SettingItem[]
}>) {
  const navigate = useNavigate()
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {items.map(item => {
          const Icon = item.icon
          return (
            <Card
              key={item.href}
              role="button"
              tabIndex={0}
              className={cn(
                'group relative cursor-pointer transition-colors',
                'hover:border-primary/40',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
              )}
              onClick={() => {
                void navigate(item.href)
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  void navigate(item.href)
                }
              }}
            >
              <ArrowRight className="text-muted-foreground absolute right-4 top-4 size-4 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
              <CardHeader>
                <div className="bg-muted text-foreground mb-2 flex size-10 items-center justify-center rounded-md">
                  <Icon className="size-5" />
                </div>
                <CardTitle className="text-base">{item.title}</CardTitle>
                <CardDescription>{item.description}</CardDescription>
              </CardHeader>
            </Card>
          )
        })}
      </div>
    </section>
  )
}

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
