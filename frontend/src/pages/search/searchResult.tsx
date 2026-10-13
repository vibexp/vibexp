import { FolderOpen } from 'lucide-react'
import { Link } from 'react-router'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  displayTitle,
  EXCERPT_PREVIEW_LENGTH,
  resourceUrl,
  TYPE_LABEL,
} from '@/pages/search/resourceUrl'
import { highlightSegments, stripMarkdown } from '@/pages/search/snippet'
import type { SearchResultItem } from '@/services/searchService'

/** Text with the query's terms marked, so a result shows why it matched. */
function Highlighted({
  text,
  query,
}: Readonly<{ text: string; query: string }>) {
  return (
    <>
      {highlightSegments(text, query).map((segment, index) =>
        segment.match ? (
          <mark
            // Segments are positional and never reorder.
            key={`${String(index)}:${segment.text}`}
            className="bg-primary/15 text-foreground rounded-sm font-medium"
          >
            {segment.text}
          </mark>
        ) : (
          segment.text
        )
      )}
    </>
  )
}

interface SearchResultCardProps {
  item: SearchResultItem
  /** The committed query, whose terms are highlighted in title and excerpt. */
  query?: string
  /** DOM id, set on the first card of a resource so citations can target it. */
  domId?: string
  /** Briefly true after an AI Summary citation scrolled to this card. */
  highlighted?: boolean
  expanded: boolean
  onToggleExpand: () => void
}

export function SearchResultCard({
  item,
  query = '',
  domId,
  highlighted = false,
  expanded,
  onToggleExpand,
}: Readonly<SearchResultCardProps>) {
  const href = resourceUrl(item)
  const title = displayTitle(item)
  // Iterate by code point (Array.from) so the preview never splits a surrogate
  // pair (e.g. emoji) at the truncation boundary.
  // The excerpt is a slice of raw markdown; it is shown as plain text.
  const plainExcerpt = stripMarkdown(item.excerpt)
  const codePoints = Array.from(plainExcerpt)
  const isTruncatable = codePoints.length > EXCERPT_PREVIEW_LENGTH
  const excerpt =
    expanded || !isTruncatable
      ? plainExcerpt
      : `${codePoints.slice(0, EXCERPT_PREVIEW_LENGTH).join('')}…`

  const header = (
    <div className="flex items-start justify-between gap-3">
      <h2 className="font-medium leading-tight">
        <Highlighted text={title} query={query} />
      </h2>
      <Badge variant="secondary" className="shrink-0">
        {TYPE_LABEL[item.type]}
      </Badge>
    </div>
  )

  return (
    <Card
      id={domId}
      data-highlighted={highlighted || undefined}
      className={cn(
        'p-4',
        href && 'transition-colors hover:bg-accent/50',
        highlighted && 'ring-primary ring-2'
      )}
    >
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

      {plainExcerpt && (
        <div className="mt-2">
          <p className="text-muted-foreground text-sm">
            <Highlighted text={excerpt} query={query} />
          </p>
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
