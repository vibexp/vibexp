import { Bot, Cpu, Mail, Shapes, SlidersHorizontal, Timer } from 'lucide-react'

import { GitHubIcon } from '@/components/icons/GitHubIcon'
import type { SettingItem } from '@/components/settings/SettingsGrid'

/**
 * The cards shown on the team settings hub, for one team (#539, populated #540).
 *
 * A sibling data module rather than a const inside `TeamSettings.tsx`, for the
 * same two reasons as `team-tabs.ts`: a `.tsx` exporting a component and a
 * value trips `react-refresh/only-export-components`, and keeping the data
 * separate makes it testable — and mockable — without rendering the page.
 *
 * Icons and copy are carried over verbatim from `pages/settings/Settings.tsx` so
 * the relocated cards are recognisably the same ones. "Artifact Types" keeps its
 * title — `customization` is only the route segment.
 *
 * #543 removes the corresponding cards from the personal hub.
 */
export function teamSettingsCardsFor(teamId: string): SettingItem[] {
  const base = `/teams/${teamId}/settings`
  return [
    {
      title: 'Search Settings',
      description: 'Choose how search results are ranked for your team.',
      icon: SlidersHorizontal,
      href: `${base}/search`,
    },
    {
      title: 'Resource Freshness',
      description: 'Flag resources your team has stopped using.',
      icon: Timer,
      href: `${base}/freshness`,
    },
    {
      title: 'Model Providers',
      description:
        'Configure OpenAI-compatible LLM providers for your AI applications.',
      icon: Bot,
      href: `${base}/model-providers`,
    },
    {
      title: 'Embedding Providers',
      description:
        'Configure embedding vector providers for your AI applications.',
      icon: Cpu,
      href: `${base}/embedding-providers`,
    },
    {
      title: 'Email Provider',
      description: "Send your team's email through your own provider.",
      icon: Mail,
      href: `${base}/email-provider`,
    },
    {
      title: 'GitHub Integration',
      description: 'Connect GitHub repositories to your team workspace.',
      icon: GitHubIcon,
      href: `${base}/integrations/github`,
    },
    {
      title: 'Artifact Types',
      description: 'Create and manage custom categories for your artifacts.',
      icon: Shapes,
      href: `${base}/customization`,
    },
  ]
}
