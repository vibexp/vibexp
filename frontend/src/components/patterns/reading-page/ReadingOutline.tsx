import { cn } from '@/lib/utils'

import type { ReadingOutline as Outline } from './useReadingOutline'

/**
 * "On this page": the body's headings as a list of links. Each scrolls to its
 * section and sets the URL hash; the section in view is marked. Real `<a
 * href="#…">` elements, so an entry can be copied or opened like any link.
 *
 * Renders nothing for a body with too few headings to need one.
 */
export function ReadingOutline({
  outline,
  onNavigate,
  className,
}: Readonly<{
  outline: Outline
  /** Called after a jump — the phone sheet closes itself with it. */
  onNavigate?: () => void
  className?: string
}>) {
  const { entries, activeId, goTo } = outline
  if (entries.length === 0) return null

  const topLevel = Math.min(...entries.map(entry => entry.level))

  return (
    <nav
      aria-label="On this page"
      data-testid="reading-outline"
      className={className}
    >
      <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
        On this page
      </p>
      <ul className="space-y-0.5 text-sm">
        {entries.map(entry => {
          const active = entry.id === activeId
          return (
            <li key={entry.id}>
              <a
                href={`#${entry.id}`}
                aria-current={active ? 'location' : undefined}
                onClick={event => {
                  event.preventDefault()
                  goTo(entry.id)
                  onNavigate?.()
                }}
                className={cn(
                  'block truncate rounded-sm border-l-2 py-1 pr-2 transition-colors',
                  entry.level > topLevel ? 'pl-6' : 'pl-3',
                  active
                    ? 'border-foreground text-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground border-transparent'
                )}
              >
                {entry.text}
              </a>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
