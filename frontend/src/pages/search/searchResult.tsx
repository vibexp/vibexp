import { FolderOpen } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  displayTitle,
  EXCERPT_PREVIEW_LENGTH,
  resourceUrl,
  TYPE_LABEL,
} from '@/pages/search/resourceUrl'
import type { SearchResultItem } from '@/services/searchService'

interface SearchResultCardProps {
  item: SearchResultItem
  expanded: boolean
  onToggleExpand: () => void
}

export function SearchResultCard({
  item,
  expanded,
  onToggleExpand,
}: SearchResultCardProps) {
  const href = resourceUrl(item)
  const title = displayTitle(item)
  // Iterate by code point (Array.from) so the preview never splits a surrogate
  // pair (e.g. emoji) at the truncation boundary.
  const codePoints = Array.from(item.excerpt)
  const isTruncatable = codePoints.length > EXCERPT_PREVIEW_LENGTH
  const excerpt =
    expanded || !isTruncatable
      ? item.excerpt
      : `${codePoints.slice(0, EXCERPT_PREVIEW_LENGTH).join('')}…`

  const header = (
    <div className="flex items-start justify-between gap-3">
      <h2 className="font-medium leading-tight">{title}</h2>
      <Badge variant="secondary" className="shrink-0">
        {TYPE_LABEL[item.type]}
      </Badge>
    </div>
  )

  return (
    <Card className={cn('p-4', href && 'transition-colors hover:bg-accent/50')}>
      {href ? (
        <Link to={href} className="block">
          {header}
        </Link>
      ) : (
        header
      )}

      {item.project_name && (
        <div className="text-muted-foreground mt-1 flex items-center gap-1 text-xs">
          <FolderOpen className="size-3.5 shrink-0" />
          <span className="truncate">{item.project_name}</span>
        </div>
      )}

      {item.excerpt && (
        <div className="mt-2">
          <p className="text-muted-foreground text-sm">{excerpt}</p>
          {isTruncatable && (
            <button
              type="button"
              onClick={onToggleExpand}
              aria-expanded={expanded}
              className="text-primary mt-1 text-xs font-medium hover:underline"
            >
              {expanded ? 'Show less' : 'Show more'}
            </button>
          )}
        </div>
      )}
    </Card>
  )
}
