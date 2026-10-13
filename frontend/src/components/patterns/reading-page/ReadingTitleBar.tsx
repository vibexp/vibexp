import { type ReactNode, useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

import type { BodyViewHandle } from './readingChrome'
import { READING_META_ROW_CLASS, READING_TITLE_CLASS } from './readingHeader'
import { ReadingToolbar } from './ReadingToolbar'
import type { ReadingAction } from './types'

interface ReadingTitleBarProps {
  title: string
  /** The title is a stand-in (an untitled memory's excerpt): drawn subdued. */
  titleSubdued?: boolean
  /** The meta line (status, type, updated) the toolbar shares a row with. */
  meta?: ReactNode
  actions: readonly ReadingAction[]
  bodyView: BodyViewHandle | null
  /** Phone layout for the toolbar. */
  compact: boolean
  /** Controls the page adds to the end of the toolbar. */
  trailing?: ReactNode
}

/**
 * A reading page's `<header>`: the title on a row of its own — the whole width
 * of the article, so a long title is not squeezed by the controls — and under
 * it the meta line with the toolbar right-aligned beside it, wrapping beneath
 * only when the two cannot share a row.
 *
 * It is STICKY. Once the reader scrolls past it, it stays under the app header
 * as a slim bar — the title on one line, the same toolbar, the meta line
 * hidden — so Edit and Copy are one click away anywhere in a long document.
 * It is one element restyled, not a second copy of the toolbar, so every
 * control and test id exists exactly once. Body headings carry a
 * `scroll-margin-top` (index.css) so an anchor never lands underneath it.
 *
 * "Stuck" is observed, not computed on scroll: a zero-height sentinel sits
 * just above the bar and leaves the viewport exactly when the bar starts
 * sticking.
 */
export function ReadingTitleBar({
  title,
  titleSubdued = false,
  meta,
  actions,
  bodyView,
  compact,
  trailing,
}: Readonly<ReadingTitleBarProps>) {
  const sentinelRef = useRef<HTMLDivElement>(null)
  const [stuck, setStuck] = useState(false)

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      ([entry]) => {
        setStuck(!entry.isIntersecting)
      },
      // The app header covers the top 3.5rem of the viewport.
      { rootMargin: '-57px 0px 0px 0px' }
    )
    observer.observe(sentinel)
    return () => {
      observer.disconnect()
    }
  }, [])

  return (
    <>
      <div ref={sentinelRef} aria-hidden="true" />
      <header
        data-testid="reading-title-bar"
        data-stuck={stuck ? 'true' : 'false'}
        className={cn(
          'bg-background sticky top-14 z-20',
          stuck && 'flex items-center gap-4 border-b py-2'
        )}
      >
        <h1
          className={cn(
            'min-w-0',
            stuck
              ? 'flex-1 truncate text-base font-semibold tracking-tight'
              : READING_TITLE_CLASS,
            titleSubdued && 'text-muted-foreground font-medium',
            // A stand-in can be a long excerpt: two lines at most.
            titleSubdued && !stuck && 'line-clamp-2'
          )}
        >
          {title}
        </h1>
        <div
          className={cn(
            // Stuck, the row dissolves: the toolbar becomes the bar's second
            // item and the meta line is hidden. (`contents` is safe here — no
            // popper is anchored on this element.)
            stuck
              ? 'contents'
              : cn(
                  READING_META_ROW_CLASS,
                  'mt-2 justify-between gap-x-4 gap-y-2'
                )
          )}
        >
          <div className={cn('min-w-0', stuck && 'hidden')}>{meta}</div>
          <ReadingToolbar
            actions={actions}
            bodyView={bodyView}
            compact={compact}
            trailing={trailing}
          />
        </div>
      </header>
    </>
  )
}
