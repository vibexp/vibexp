import { type ReactNode, useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

import type { BodyViewHandle } from './readingChrome'
import { ReadingToolbar } from './ReadingToolbar'
import type { ReadingAction } from './types'

interface ReadingTitleBarProps {
  title: string
  /** The title is a stand-in (an untitled memory's excerpt): drawn subdued. */
  titleSubdued?: boolean
  actions: readonly ReadingAction[]
  bodyView: BodyViewHandle | null
  /** Phone layout for the toolbar. */
  compact: boolean
  /** Controls the page adds to the end of the toolbar. */
  trailing?: ReactNode
}

/**
 * A reading page's `<header>`: the title with the toolbar beside it.
 *
 * It is STICKY. Once the reader scrolls past it, it stays under the app header
 * as a slim bar — the title on one line, the same toolbar — so Edit and Copy
 * are one click away anywhere in a long document. It is one element, not a
 * second copy of the toolbar, so every control and test id exists exactly
 * once. Body headings carry a `scroll-margin-top` (index.css) so an anchor
 * never lands underneath it.
 *
 * "Stuck" is observed, not computed on scroll: a zero-height sentinel sits
 * just above the bar and leaves the viewport exactly when the bar starts
 * sticking.
 */
export function ReadingTitleBar({
  title,
  titleSubdued = false,
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
          'bg-background sticky top-14 z-20 flex flex-wrap justify-between gap-x-4 gap-y-2',
          stuck ? 'items-center border-b py-2' : 'items-start pb-3'
        )}
      >
        <h1
          className={cn(
            'min-w-0 font-semibold tracking-tight',
            // Stuck: one line beside the toolbar. Otherwise the full title —
            // on its own row on a phone, where the toolbar wraps beneath it.
            stuck
              ? 'flex-1 truncate text-base'
              : 'basis-full text-2xl md:flex-1 md:basis-0 md:text-3xl',
            titleSubdued && 'text-muted-foreground font-medium',
            // A stand-in can be a long excerpt: two lines at most.
            titleSubdued && !stuck && 'line-clamp-2'
          )}
        >
          {title}
        </h1>
        <ReadingToolbar
          actions={actions}
          bodyView={bodyView}
          compact={compact}
          trailing={trailing}
        />
      </header>
    </>
  )
}
