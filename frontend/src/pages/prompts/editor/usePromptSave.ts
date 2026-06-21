import { useState } from 'react'

import { toast } from '@/lib/toast'
import { promptService } from '@/services/promptService'
import type { Prompt } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

import type { PromptFormData } from './types'

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

  const save = async (formData: PromptFormData): Promise<string | null> => {
    if (!teamId) {
      toast.error('No team selected')
      return null
    }

    try {
      setSaving(true)

      const payload = {
        name: formData.name,
        slug: formData.slug,
        description: formData.description,
        body: formData.body,
        status: formData.status,
        mcp_expose: formData.mcp_expose,
        labels: formData.labels,
        project_id: formData.project_id,
      }

      if (prompt) {
        await promptService.updatePrompt(teamId, prompt.slug, payload)
        trackEvent({
          event: ANALYTICS_EVENTS.PROMPT_UPDATED,
          properties: {
            prompt_id: prompt.slug,
            prompt_title: formData.name,
            prompt_type: formData.status,
            action_context: 'update',
          },
        })
        toast.success('Prompt updated successfully')
      } else {
        await promptService.createPrompt(teamId, payload)
        trackEvent({
          event: ANALYTICS_EVENTS.PROMPT_CREATED,
          properties: {
            prompt_id: formData.slug,
            prompt_title: formData.name,
            prompt_type: formData.status,
            action_context: 'create',
          },
        })
        toast.success('Prompt created successfully')
      }
      return formData.slug
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to save prompt'))
      return null
    } finally {
      setSaving(false)
    }
  }

  return { saving, save }
}
