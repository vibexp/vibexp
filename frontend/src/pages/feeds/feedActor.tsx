import type { ReactNode } from 'react'

import { resolveAiAssistantIcon } from '@/lib/aiAssistantIcon'
import { assistantColor, assistantInitial } from '@/lib/avatar'
import type { TeamMember } from '@/types/team'

// Sentinel value the FeedPostComposer writes to `ai_assistant_name` when a
// human posts a feed item. Centralised here so the rendering layer can
// recognise user-authored items without importing the composer.
export const USER_POST_ASSISTANT_NAME = 'User Post'

export interface FeedActorSource {
  ai_assistant_name?: string | null
  posted_by_user_id?: string | null
}

export interface FeedActor {
  isAi: boolean
  displayName: string
  member?: TeamMember
  aiAssistantName?: string
}

export function resolveFeedActor(
  source: FeedActorSource,
  member: TeamMember | undefined
): FeedActor {
  const aiName = source.ai_assistant_name?.trim() ?? ''
  const isAi =
    aiName !== '' &&
    aiName.toLowerCase() !== USER_POST_ASSISTANT_NAME.toLowerCase()

  if (isAi) {
    return {
      isAi: true,
      displayName: aiName,
      aiAssistantName: aiName,
    }
  }

  return {
    isAi: false,
    displayName: member?.name ?? 'Unknown user',
    member,
  }
}

const SIZE_CLASSES: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'h-8 w-8 text-xs',
  md: 'h-10 w-10 text-sm',
  lg: 'h-12 w-12 text-base',
}

const ICON_SIZE_CLASSES: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'h-5 w-5',
  md: 'h-6 w-6',
  lg: 'h-7 w-7',
}

interface FeedActorAvatarProps {
  actor: FeedActor
  size?: 'sm' | 'md' | 'lg'
}

export function FeedActorAvatar({
  actor,
  size = 'md',
}: FeedActorAvatarProps): ReactNode {
  if (actor.isAi && actor.aiAssistantName) {
    const icon = resolveAiAssistantIcon(actor.aiAssistantName)
    return (
      <div
        className={`${SIZE_CLASSES[size]} shrink-0 rounded-full bg-white border border-border flex items-center justify-center overflow-hidden`}
        data-testid={`ai-assistant-icon-${icon.provider}`}
        aria-hidden="true"
      >
        <img
          src={icon.src}
          alt={icon.alt}
          className={`${ICON_SIZE_CLASSES[size]} object-contain`}
        />
      </div>
    )
  }

  const name = actor.member?.name ?? ''
  const bg = name !== '' ? assistantColor(name) : 'bg-muted-foreground'
  const initial = name !== '' ? assistantInitial(name) : '?'
  return (
    <div
      className={`${SIZE_CLASSES[size]} shrink-0 rounded-full ${bg} flex items-center justify-center font-semibold text-white`}
      aria-hidden="true"
    >
      {initial}
    </div>
  )
}
