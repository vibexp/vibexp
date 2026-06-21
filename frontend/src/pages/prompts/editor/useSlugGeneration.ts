import { useCallback, useState } from 'react'

import { promptService } from '@/services/promptService'
import type { Team } from '@/types'

export function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function useSlugGeneration(
  currentTeam: Team | null,
  currentPromptSlug: string | undefined
) {
  const [isCheckingSlug, setIsCheckingSlug] = useState(false)

  const generateUniqueSlug = useCallback(
    async (baseSlug: string): Promise<string> => {
      if (!baseSlug || !currentTeam) return ''

      setIsCheckingSlug(true)
      try {
        const response = await promptService.getPrompts(currentTeam.id, {
          limit: 1000,
        })
        const existingSlugs = response.data.prompts
          .filter(p => p.slug !== currentPromptSlug)
          .map(p => p.slug)

        if (!existingSlugs.includes(baseSlug)) return baseSlug

        const randomSuffix = () => {
          const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
          let r = ''
          for (let i = 0; i < 4; i++) {
            r += chars.charAt(Math.floor(Math.random() * chars.length))
          }
          return r
        }

        let attempts = 0
        let candidate = baseSlug
        while (existingSlugs.includes(candidate) && attempts < 10) {
          candidate = `${baseSlug}-${randomSuffix()}`
          attempts++
        }
        return candidate
      } catch {
        return baseSlug
      } finally {
        setIsCheckingSlug(false)
      }
    },
    [currentTeam, currentPromptSlug]
  )

  return { isCheckingSlug, generateUniqueSlug }
}
