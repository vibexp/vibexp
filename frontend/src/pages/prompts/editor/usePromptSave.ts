import { useState } from 'react'

import { toast } from '@/lib/toast'
import type { CreatePromptRequest, Prompt } from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

interface UsePromptSaveArgs {
  teamId: string | undefined
  prompt: Prompt | null
  trackEvent: (event: {
    event: string
    properties?: Record<string, unknown>
  }) => void
}

export function usePromptSave({
  teamId,
  prompt,
  trackEvent,
}: UsePromptSaveArgs) {
  const [saving, setSaving] = useState(false)

  // Takes the request body rather than a form-shaped object: since #915 the
  // form's parsed values are mapped to it by `toPromptRequest`, so this hook
  // owns the call, the analytics and the toasts and nothing about field shape.
  const save = async (payload: CreatePromptRequest): Promise<string | null> => {
    if (!teamId) {
      toast.error('No team selected')
      return null
    }

    try {
      setSaving(true)

      if (prompt) {
        await promptService.updatePrompt(teamId, prompt.slug, payload)
        trackEvent({
          event: ANALYTICS_EVENTS.PROMPT_UPDATED,
          properties: {
            prompt_id: prompt.slug,
            prompt_title: payload.name,
            prompt_type: payload.status,
            action_context: 'update',
          },
        })
        toast.success('Prompt updated successfully')
      } else {
        await promptService.createPrompt(teamId, payload)
        trackEvent({
          event: ANALYTICS_EVENTS.PROMPT_CREATED,
          properties: {
            prompt_id: payload.slug,
            prompt_title: payload.name,
            prompt_type: payload.status,
            action_context: 'create',
          },
        })
        toast.success('Prompt created successfully')
      }
      return payload.slug
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to save prompt'))
      return null
    } finally {
      setSaving(false)
    }
  }

  return { saving, save }
}
