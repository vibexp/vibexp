import { Share2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'

/**
 * The prompt header's "Shared" badge — one component so the reading page and
 * the edit page's header (#1179) cannot drift apart.
 */
export function SharedBadge() {
  return (
    <Badge variant="secondary" className="gap-1">
      <Share2 className="size-3" />
      Shared
    </Badge>
  )
}
